package model

import (
	"time"
)

// ReputationTier represents the tier level of a provider's reputation
type ReputationTier string

const (
	TierBronze   ReputationTier = "BRONZE"
	TierSilver   ReputationTier = "SILVER"
	TierGold     ReputationTier = "GOLD"
	TierPlatinum ReputationTier = "PLATINUM"
)

// ReputationScore represents the computed reputation for a provider
type ReputationScore struct {
	ProviderID          string                  `json:"provider_id" bson:"provider_id"`
	TenantID            string                  `json:"tenant_id" bson:"tenant_id"`
	OverallScore        float64                 `json:"overall_score" bson:"overall_score"`
	ReputationTier      ReputationTier          `json:"reputation_tier" bson:"reputation_tier"`
	TransactionScore    float64                 `json:"transaction_score" bson:"transaction_score"`
	SuccessRate         float64                 `json:"success_rate" bson:"success_rate"`
	VolumeScore         float64                 `json:"volume_score" bson:"volume_score"`
	ConsistencyScore    float64                 `json:"consistency_score" bson:"consistency_score"`
	CertificationBonus  float64                 `json:"certification_bonus" bson:"certification_bonus"`
	TotalContracts      int64                   `json:"total_contracts" bson:"total_contracts"`
	SuccessfulContracts int64                   `json:"successful_contracts" bson:"successful_contracts"`
	FailedContracts     int64                   `json:"failed_contracts" bson:"failed_contracts"`
	DisputedContracts   int64                   `json:"disputed_contracts" bson:"disputed_contracts"`
	CategoryStats       map[string]CategoryStat `json:"category_stats" bson:"category_stats"`
	ActiveCertificates  int                     `json:"active_certificates" bson:"active_certificates"`
	FlaggedForReview    bool                    `json:"flagged_for_review" bson:"flagged_for_review"`
	FlagReason          string                  `json:"flag_reason,omitempty" bson:"flag_reason,omitempty"`
	LastCalculatedAt    time.Time               `json:"last_calculated_at" bson:"last_calculated_at"`
	UpdatedAt           time.Time               `json:"updated_at" bson:"updated_at"`
}

// CategoryStat represents per-category statistics for a provider
type CategoryStat struct {
	Category            string  `json:"category" bson:"category"`
	TotalContracts      int64   `json:"total_contracts" bson:"total_contracts"`
	SuccessfulContracts int64   `json:"successful_contracts" bson:"successful_contracts"`
	SuccessRate         float64 `json:"success_rate" bson:"success_rate"`
	AverageRating       float64 `json:"average_rating" bson:"average_rating"`
}
