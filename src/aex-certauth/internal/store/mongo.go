package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parlakisik/agent-exchange/aex-certauth/internal/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoCertAuthStore struct {
	client       *mongo.Client
	certificates *mongo.Collection
	requests     *mongo.Collection
	reputations  *mongo.Collection
	crls         *mongo.Collection
}

func NewMongoCertAuthStore(client *mongo.Client, dbName string) *MongoCertAuthStore {
	db := client.Database(dbName)
	return &MongoCertAuthStore{
		client:       client,
		certificates: db.Collection("certificates"),
		requests:     db.Collection("certificate_requests"),
		reputations:  db.Collection("reputations"),
		crls:         db.Collection("crls"),
	}
}

func (s *MongoCertAuthStore) EnsureIndexes(ctx context.Context) error {
	// Certificate indexes
	_, err := s.certificates.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "certificate_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{
				{Key: "provider_id", Value: 1},
				{Key: "status", Value: 1},
			},
		},
		{
			Keys: bson.D{{Key: "tenant_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "claims.capability", Value: "text"}},
		},
	})
	if err != nil {
		return err
	}

	// Certificate request indexes
	_, err = s.requests.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "request_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "provider_id", Value: 1}, {Key: "created_at", Value: -1}},
		},
	})
	if err != nil {
		return err
	}

	// Reputation indexes
	_, err = s.reputations.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "provider_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "overall_score", Value: -1}},
		},
	})
	if err != nil {
		return err
	}

	// CRL indexes
	_, err = s.crls.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: -1}},
	})

	return err
}

// Certificate Requests

func (s *MongoCertAuthStore) CreateCertificateRequest(ctx context.Context, req model.CertificateRequest) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.requests.InsertOne(ctx, req)
	return err
}

func (s *MongoCertAuthStore) GetCertificateRequest(ctx context.Context, requestID string) (model.CertificateRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var req model.CertificateRequest
	err := s.requests.FindOne(ctx, bson.M{"request_id": requestID}).Decode(&req)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.CertificateRequest{}, fmt.Errorf("certificate request: %w", ErrNotFound)
		}
		return model.CertificateRequest{}, err
	}
	return req, nil
}

func (s *MongoCertAuthStore) UpdateCertificateRequest(ctx context.Context, requestID string, status string, reviewedBy string, reviewNote string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"status":      status,
			"reviewed_by": reviewedBy,
			"review_note": reviewNote,
			"updated_at":  time.Now().UTC(),
		},
	}
	result, err := s.requests.UpdateOne(ctx, bson.M{"request_id": requestID}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("certificate request: %w", ErrNotFound)
	}
	return nil
}

// Certificates

func (s *MongoCertAuthStore) CreateCertificate(ctx context.Context, cert model.AgentCertificate) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.certificates.InsertOne(ctx, cert)
	return err
}

func (s *MongoCertAuthStore) GetCertificate(ctx context.Context, certID string) (model.AgentCertificate, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var cert model.AgentCertificate
	err := s.certificates.FindOne(ctx, bson.M{"certificate_id": certID}).Decode(&cert)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.AgentCertificate{}, fmt.Errorf("certificate: %w", ErrNotFound)
		}
		return model.AgentCertificate{}, err
	}
	return cert, nil
}

func (s *MongoCertAuthStore) GetCertificatesByProvider(ctx context.Context, providerID string) ([]model.AgentCertificate, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	cur, err := s.certificates.Find(ctx, bson.M{"provider_id": providerID}, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var certs []model.AgentCertificate
	if err := cur.All(ctx, &certs); err != nil {
		return nil, err
	}
	return certs, nil
}

func (s *MongoCertAuthStore) UpdateCertificateStatus(ctx context.Context, certID string, status string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"updated_at": time.Now().UTC(),
		},
	}
	result, err := s.certificates.UpdateOne(ctx, bson.M{"certificate_id": certID}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("certificate: %w", ErrNotFound)
	}
	return nil
}

func (s *MongoCertAuthStore) RevokeCertificate(ctx context.Context, certID string, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	update := bson.M{
		"$set": bson.M{
			"status":            model.CertStatusRevoked,
			"revoked_at":        now,
			"revocation_reason": reason,
			"updated_at":        now,
		},
	}
	result, err := s.certificates.UpdateOne(ctx, bson.M{"certificate_id": certID}, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("certificate: %w", ErrNotFound)
	}
	return nil
}

func (s *MongoCertAuthStore) SearchCertificates(ctx context.Context, filters CertificateFilters) ([]model.AgentCertificate, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	filter := bson.M{}
	if filters.ProviderID != "" {
		filter["provider_id"] = filters.ProviderID
	}
	if filters.TenantID != "" {
		filter["tenant_id"] = filters.TenantID
	}
	if filters.CertificateType != "" {
		filter["certificate_type"] = filters.CertificateType
	}
	if filters.Status != "" {
		filter["status"] = filters.Status
	}
	if filters.Category != "" {
		filter["claims.category"] = filters.Category
	}
	if filters.Capability != "" {
		filter["$text"] = bson.M{"$search": filters.Capability}
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}})
	if filters.Limit > 0 {
		opts.SetLimit(int64(filters.Limit))
	}
	if filters.Offset > 0 {
		opts.SetSkip(int64(filters.Offset))
	}

	cur, err := s.certificates.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var certs []model.AgentCertificate
	if err := cur.All(ctx, &certs); err != nil {
		return nil, err
	}
	return certs, nil
}

// Reputation

func (s *MongoCertAuthStore) UpsertReputation(ctx context.Context, reputation model.ReputationScore) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := s.reputations.ReplaceOne(
		ctx,
		bson.M{"provider_id": reputation.ProviderID},
		reputation,
		options.Replace().SetUpsert(true),
	)
	return err
}

func (s *MongoCertAuthStore) GetReputation(ctx context.Context, providerID string) (model.ReputationScore, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	var rep model.ReputationScore
	err := s.reputations.FindOne(ctx, bson.M{"provider_id": providerID}).Decode(&rep)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.ReputationScore{}, fmt.Errorf("reputation: %w", ErrNotFound)
		}
		return model.ReputationScore{}, err
	}
	return rep, nil
}

func (s *MongoCertAuthStore) GetLeaderboard(ctx context.Context, limit int, offset int) ([]model.ReputationScore, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "overall_score", Value: -1}})
	if limit > 0 {
		opts.SetLimit(int64(limit))
	}
	if offset > 0 {
		opts.SetSkip(int64(offset))
	}

	cur, err := s.reputations.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	var scores []model.ReputationScore
	if err := cur.All(ctx, &scores); err != nil {
		return nil, err
	}
	return scores, nil
}

// CRL

func (s *MongoCertAuthStore) SaveCRL(ctx context.Context, crl model.CRL) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := s.crls.InsertOne(ctx, crl)
	return err
}

func (s *MongoCertAuthStore) GetLatestCRL(ctx context.Context) (model.CRL, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	opts := options.FindOne().SetSort(bson.D{{Key: "created_at", Value: -1}})
	var crl model.CRL
	err := s.crls.FindOne(ctx, bson.M{}, opts).Decode(&crl)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return model.CRL{}, fmt.Errorf("CRL: %w", ErrNotFound)
		}
		return model.CRL{}, err
	}
	return crl, nil
}

func (s *MongoCertAuthStore) CountActiveCertificates(ctx context.Context, providerID string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	count, err := s.certificates.CountDocuments(ctx, bson.M{
		"provider_id": providerID,
		"status":      model.CertStatusActive,
	})
	if err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *MongoCertAuthStore) ListAllProviderIDs(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	results, err := s.reputations.Distinct(ctx, "provider_id", bson.M{})
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(results))
	for _, r := range results {
		if id, ok := r.(string); ok {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *MongoCertAuthStore) Close() error {
	// MongoDB client is shared, no need to close here
	return nil
}
