package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// RateLimiter implements Redis-backed fixed-window rate limiting.
// Each tenant gets a counter key in Redis with a TTL equal to the window size.
// If Redis is unavailable the limiter fails open (allows the request).
type RateLimiter struct {
	rdb        *redis.Client
	limit      int
	windowSize time.Duration
}

// NewRateLimiter creates a Redis-backed rate limiter.
// redisURL should be a valid Redis connection string (e.g. "redis://localhost:6379").
func NewRateLimiter(redisURL string, limitPerMinute int) *RateLimiter {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		log.Printf("WARN: ratelimit: invalid REDIS_URL %q, rate limiting will fail open: %v", redisURL, err)
		return &RateLimiter{
			rdb:        nil,
			limit:      limitPerMinute,
			windowSize: time.Minute,
		}
	}

	rdb := redis.NewClient(opts)

	// Quick connectivity check – non-blocking; we just log on failure.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Printf("WARN: ratelimit: cannot reach Redis at %q, rate limiting will fail open until reconnect: %v", redisURL, err)
	}

	return &RateLimiter{
		rdb:        rdb,
		limit:      limitPerMinute,
		windowSize: time.Minute,
	}
}

// Allow checks whether a request for the given key (tenant) should be allowed.
// It returns whether the request is allowed, the number of remaining requests in
// the current window, and the time at which the window resets.
//
// Algorithm: fixed-window using INCR + EXPIRE.
//   - Key format: "ratelimit:<tenantID>:<windowStart>"
//   - On the first request in a window the key is created with EXPIRE = windowSize.
//   - Subsequent requests increment the counter and compare against the limit.
func (rl *RateLimiter) Allow(key string) (allowed bool, remaining int, resetAt time.Time) {
	now := time.Now()
	windowStart := now.Truncate(rl.windowSize)
	resetAt = windowStart.Add(rl.windowSize)

	// Fail open when Redis is not configured.
	if rl.rdb == nil {
		return true, rl.limit, resetAt
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	redisKey := "ratelimit:" + key + ":" + strconv.FormatInt(windowStart.Unix(), 10)

	count, err := rl.rdb.Incr(ctx, redisKey).Result()
	if err != nil {
		log.Printf("WARN: ratelimit: Redis INCR failed for key %q, failing open: %v", redisKey, err)
		return true, rl.limit, resetAt
	}

	// Set expiry on the first request for this window.
	if count == 1 {
		ttl := time.Until(resetAt) + time.Second // small buffer to avoid premature eviction
		if err := rl.rdb.Expire(ctx, redisKey, ttl).Err(); err != nil {
			log.Printf("WARN: ratelimit: Redis EXPIRE failed for key %q: %v", redisKey, err)
		}
	}

	remaining = rl.limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	if int(count) > rl.limit {
		return false, 0, resetAt
	}

	return true, remaining, resetAt
}

// RateLimit returns an HTTP middleware that enforces per-tenant rate limits.
func RateLimit(limiter *RateLimiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tenantID := GetTenantID(r.Context())
			if tenantID == "" {
				tenantID = "anonymous"
			}

			allowed, remaining, resetAt := limiter.Allow(tenantID)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limiter.limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))

			if !allowed {
				retryAfter := int(time.Until(resetAt).Seconds())
				if retryAfter < 1 {
					retryAfter = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]any{
						"code":       "rate_limit_exceeded",
						"message":    "Rate limit exceeded. Please retry after " + strconv.Itoa(retryAfter) + " seconds.",
						"request_id": GetRequestID(r.Context()),
					},
				})
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
