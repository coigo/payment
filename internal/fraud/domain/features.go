package domain

import (
	"time"
)

type Attempt struct {
	PaymentId string
	AccountId string
	CardFingerPrint string
	AmountCents int64
	Currency string
	ClientIP string
	MerchantCategory string
	OutcurredAt time.Time

	CountryCode string
	VelocityLastMin int
	RistScore float64
	Reasons []string
	Degraded bool
}

const (
	FeatureAmountCents = 0
	FeatureVelocityLastMin = 1
	FeatureNightTime = 2
	FeatureForgeinCountry = 3
	FeatureHighRiskMMC = 4
	FeatureCount = 5
)

var HighRiskMerchantCategories = map[string]bool {
	"gambling": true,
	"crypto": true,
	"gift_cards": true,
	"money_transfer": true,
}

func FeatureVector (a Attempt) []float64 {
	v := make([]float64, FeatureCount)
	v[FeatureAmountCents] = float64(a.AmountCents)
	v[FeatureVelocityLastMin] = float64(a.VelocityLastMin)

	hour := a.OutcurredAt.Hour()
	if hour < 6 {
		v[FeatureNightTime] = 1
	}
	if a.CountryCode != "" && a.CountryCode != "BR" {
		v[FeatureForgeinCountry] = 1
	}
	if HighRiskMerchantCategories[a.MerchantCategory] {
		v[FeatureHighRiskMMC] = 1
	}

	return v
}

type Verdict string

const (
	VerdictApprove = "approve"
	VerdictDeny = "deny"
	VerdictManualReview = "manual_review"
)

const (
	ApproveBelow = 0.35
	DenyAbove = 0.75
)

func VeredictFromScore(score float64) Verdict {
	switch {
		case score < ApproveBelow: return VerdictApprove
		case score > DenyAbove: return VerdictDeny
		default: return VerdictManualReview
	}
}