package service

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/clients"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/model"
	"github.com/parlakisik/agent-exchange/aex-certauth/internal/store"
	"github.com/parlakisik/agent-exchange/internal/events"
)

// ReputationService computes and manages provider reputation scores using
// data from the trust-broker and local certificate counts.
type ReputationService struct {
	store       store.CertAuthStore
	trustClient *clients.TrustBrokerClient
	publisher   *events.Publisher
}

// NewReputationService creates a new ReputationService.
func NewReputationService(
	st store.CertAuthStore,
	trustClient *clients.TrustBrokerClient,
	publisher *events.Publisher,
) *ReputationService {
	return &ReputationService{
		store:       st,
		trustClient: trustClient,
		publisher:   publisher,
	}
}

// CalculateReputation fetches trust data from the trust-broker, computes the
// weighted reputation score, applies anti-gaming safeguards, and stores the
// result.
func (s *ReputationService) CalculateReputation(ctx context.Context, providerID string) (model.ReputationScore, error) {
	trustData, err := s.trustClient.GetTrustRecord(ctx, providerID)
	if err != nil {
		return model.ReputationScore{}, err
	}

	activeCerts, err := s.store.CountActiveCertificates(ctx, providerID)
	if err != nil {
		slog.WarnContext(ctx, "failed to count active certificates, defaulting to 0",
			"provider_id", providerID,
			"error", err,
		)
		activeCerts = 0
	}

	// Fetch previous reputation for anti-gaming comparison.
	var previous *model.ReputationScore
	prev, err := s.store.GetReputation(ctx, providerID)
	if err == nil {
		previous = &prev
	}

	score := s.computeScore(trustData, activeCerts)
	score.ProviderID = providerID

	// Build category stats from trust-broker breakdowns.
	score.CategoryStats = make(map[string]model.CategoryStat, len(trustData.CategoryBreakdowns))
	for cat, detail := range trustData.CategoryBreakdowns {
		score.CategoryStats[cat] = model.CategoryStat{
			Category:            detail.Category,
			TotalContracts:      detail.TotalContracts,
			SuccessfulContracts: detail.SuccessfulContracts,
			SuccessRate:         detail.SuccessRate,
			AverageRating:       detail.AverageRating,
		}
	}

	// Anti-gaming safeguards
	s.applyAntiGaming(&score, previous)

	now := time.Now().UTC()
	score.LastCalculatedAt = now
	score.UpdatedAt = now

	if err := s.store.UpsertReputation(ctx, score); err != nil {
		return model.ReputationScore{}, err
	}

	// Publish reputation-updated event.
	var prevScore float64
	if previous != nil {
		prevScore = previous.OverallScore
	}
	_ = s.publisher.Publish(ctx, events.EventReputationUpdated, map[string]any{
		"provider_id":    providerID,
		"previous_score": prevScore,
		"new_score":      score.OverallScore,
		"tier":           string(score.ReputationTier),
	})

	return score, nil
}

// GetReputation returns the cached reputation score for a provider.
func (s *ReputationService) GetReputation(ctx context.Context, providerID string) (model.ReputationScore, error) {
	return s.store.GetReputation(ctx, providerID)
}

// GetLeaderboard returns the top agents ordered by overall score.
func (s *ReputationService) GetLeaderboard(ctx context.Context, limit, offset int) ([]model.ReputationScore, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	return s.store.GetLeaderboard(ctx, limit, offset)
}

// RecalculateAll recalculates reputation for every known provider. This is
// intended to be called as a background job.
func (s *ReputationService) RecalculateAll(ctx context.Context) error {
	providerIDs, err := s.store.ListAllProviderIDs(ctx)
	if err != nil {
		return err
	}

	slog.InfoContext(ctx, "starting batch reputation recalculation",
		"provider_count", len(providerIDs),
	)

	var failures int
	for _, pid := range providerIDs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := s.CalculateReputation(ctx, pid); err != nil {
			failures++
			slog.WarnContext(ctx, "failed to recalculate reputation",
				"provider_id", pid,
				"error", err,
			)
		}
	}

	slog.InfoContext(ctx, "batch reputation recalculation complete",
		"provider_count", len(providerIDs),
		"failures", failures,
	)
	return nil
}

// computeScore applies the weighted reputation formula:
//
//	OverallScore = (0.35 * TransactionScore) +
//	              (0.25 * SuccessRate) +
//	              (0.15 * VolumeScore) +
//	              (0.15 * ConsistencyScore) +
//	              (0.10 * CertificationBonus)
func (s *ReputationService) computeScore(data clients.TrustData, activeCerts int) model.ReputationScore {
	transactionScore := clamp01(data.AverageScore)

	var successRate float64
	if data.TotalContracts > 0 {
		successRate = float64(data.SuccessfulContracts) / float64(data.TotalContracts)
	}
	successRate = clamp01(successRate)

	// VolumeScore: min(1.0, total_contracts / 500)
	volumeScore := clamp01(float64(data.TotalContracts) / 500.0)

	// ConsistencyScore: 1.0 - stddev(30-day rolling success rates)
	// Approximate using per-category success rates as a proxy for rolling
	// windows when detailed time-series data is not available.
	consistencyScore := computeConsistencyScore(data)

	// CertificationBonus: min(0.10, active_certs * 0.05)
	certBonus := math.Min(0.10, float64(activeCerts)*0.05)

	score := model.ReputationScore{
		TransactionScore:    transactionScore,
		SuccessRate:         successRate,
		VolumeScore:         volumeScore,
		ConsistencyScore:    consistencyScore,
		CertificationBonus:  certBonus,
		TotalContracts:      data.TotalContracts,
		SuccessfulContracts: data.SuccessfulContracts,
		FailedContracts:     data.FailedContracts,
		DisputedContracts:   data.DisputedContracts,
		ActiveCertificates:  activeCerts,
	}

	overall := computeWeightedScore(score)
	tier := assignTier(overall, data.TotalContracts)

	score.OverallScore = overall
	score.ReputationTier = tier

	return score
}

// computeWeightedScore applies the canonical reputation formula to a score's
// component fields. This is the single source of truth for the weights.
func computeWeightedScore(s model.ReputationScore) float64 {
	return clamp01(
		(0.35 * s.TransactionScore) +
			(0.25 * s.SuccessRate) +
			(0.15 * s.VolumeScore) +
			(0.15 * s.ConsistencyScore) +
			(0.10 * s.CertificationBonus),
	)
}

// computeConsistencyScore calculates 1.0 - stddev of per-category success
// rates as a proxy for rolling consistency.
func computeConsistencyScore(data clients.TrustData) float64 {
	if len(data.CategoryBreakdowns) == 0 {
		return 0.5 // neutral default when no category data is available
	}

	rates := make([]float64, 0, len(data.CategoryBreakdowns))
	for _, cat := range data.CategoryBreakdowns {
		if cat.TotalContracts > 0 {
			rates = append(rates, cat.SuccessRate)
		}
	}

	if len(rates) == 0 {
		return 0.5
	}

	// Calculate mean
	var sum float64
	for _, r := range rates {
		sum += r
	}
	mean := sum / float64(len(rates))

	// Calculate standard deviation
	var variance float64
	for _, r := range rates {
		diff := r - mean
		variance += diff * diff
	}
	variance /= float64(len(rates))
	stddev := math.Sqrt(variance)

	return clamp01(1.0 - stddev)
}

// assignTier determines the reputation tier based on score and volume
// thresholds.
//
//	PLATINUM: score >= 0.9  AND total_contracts >= 200
//	GOLD:     score >= 0.75 AND total_contracts >= 50
//	SILVER:   score >= 0.5  AND total_contracts >= 10
//	BRONZE:   everything else
func assignTier(score float64, totalContracts int64) model.ReputationTier {
	if score >= 0.9 && totalContracts >= 200 {
		return model.TierPlatinum
	}
	if score >= 0.75 && totalContracts >= 50 {
		return model.TierGold
	}
	if score >= 0.5 && totalContracts >= 10 {
		return model.TierSilver
	}
	return model.TierBronze
}

// applyAntiGaming applies safeguards against gaming the reputation system.
func (s *ReputationService) applyAntiGaming(score *model.ReputationScore, previous *model.ReputationScore) {
	// Safeguard 1: If volume jumps more than 3x in a week, flag for review.
	if previous != nil && previous.TotalContracts > 0 {
		ratio := float64(score.TotalContracts) / float64(previous.TotalContracts)
		if ratio > 3.0 {
			score.FlaggedForReview = true
			score.FlagReason = "volume increased more than 3x since last calculation"
		}
	}

	// Safeguard 2: If success rate is exactly 1.0 with high volume, add
	// consistency penalty. A perfect score with significant volume is
	// statistically unlikely and may indicate self-dealing.
	if score.SuccessRate == 1.0 && score.TotalContracts > 50 {
		penalty := 0.05
		score.ConsistencyScore = clamp01(score.ConsistencyScore - penalty)

		// Recompute overall with the adjusted consistency.
		score.OverallScore = computeWeightedScore(*score)

		// Reassign tier with the adjusted score.
		score.ReputationTier = assignTier(score.OverallScore, score.TotalContracts)

		if !score.FlaggedForReview {
			score.FlaggedForReview = true
			score.FlagReason = "perfect success rate with high volume"
		}
	}
}

func clamp01(v float64) float64 {
	if math.IsNaN(v) {
		return 0.0
	}
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
