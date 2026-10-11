package domain_test

import (
	"testing"
	"time"

	"github.com/coigo/payment-processor-course/internal/fraud/domain"
)

func TestFeatureVector(t *testing.T) {
	tests := []struct {
		name     string
		attempt  domain.Attempt
		wantLen  int
		checkIdx map[int]float64
	}{
		{
			name: "basic attempt no flags",
			attempt: domain.Attempt{
				AmountCents:      10000,
				VelocityLastMin:  2,
				OutcurredAt:      time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode:      "BR",
				MerchantCategory: "retail",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureAmountCents: 10000, domain.FeatureVelocityLastMin: 2},
		},
		{
			name: "night time hour 5",
			attempt: domain.Attempt{
				AmountCents:      100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 5, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "retail",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureNightTime: 1},
		},
		{
			name: "night time hour 6 is not night",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 6, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "retail",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureNightTime: 0},
		},
		{
			name: "foreign country US",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "US", MerchantCategory: "retail",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureForgeinCountry: 1},
		},
		{
			name: "BR country not foreign",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "retail",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureForgeinCountry: 0},
		},
		{
			name: "empty country not foreign",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "", MerchantCategory: "retail",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureForgeinCountry: 0},
		},
		{
			name: "high risk gambling",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "gambling",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureHighRiskMMC: 1},
		},
		{
			name: "high risk crypto",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "crypto",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureHighRiskMMC: 1},
		},
		{
			name: "high risk gift_cards",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "gift_cards",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureHighRiskMMC: 1},
		},
		{
			name: "high risk money_transfer",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "money_transfer",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureHighRiskMMC: 1},
		},
		{
			name: "retail not high risk",
			attempt: domain.Attempt{
				AmountCents: 100, VelocityLastMin: 0,
				OutcurredAt: time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				CountryCode: "BR", MerchantCategory: "retail",
			},
			wantLen:  domain.FeatureCount,
			checkIdx: map[int]float64{domain.FeatureHighRiskMMC: 0},
		},
		{
			name: "all flags set",
			attempt: domain.Attempt{
				AmountCents: 500000, VelocityLastMin: 15,
				OutcurredAt: time.Date(2025, 1, 1, 3, 0, 0, 0, time.UTC),
				CountryCode: "US", MerchantCategory: "gambling",
			},
			wantLen: domain.FeatureCount,
			checkIdx: map[int]float64{
				domain.FeatureAmountCents:     500000,
				domain.FeatureVelocityLastMin: 15,
				domain.FeatureNightTime:       1,
				domain.FeatureForgeinCountry:  1,
				domain.FeatureHighRiskMMC:     1,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.FeatureVector(tt.attempt)
			if len(got) != tt.wantLen {
				t.Errorf("len = %d, want %d", len(got), tt.wantLen)
			}
			for idx, wantVal := range tt.checkIdx {
				if got[idx] != wantVal {
					t.Errorf("[%d] = %v, want %v", idx, got[idx], wantVal)
				}
			}
		})
	}
}

func TestVeredictFromScore(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  domain.Verdict
	}{
		{name: "score 0", score: 0, want: domain.VerdictApprove},
		{name: "score 0.34 below ApproveBelow", score: 0.34, want: domain.VerdictApprove},
		{name: "score exactly ApproveBelow 0.35", score: 0.35, want: domain.VerdictManualReview},
		{name: "score 0.36 above ApproveBelow", score: 0.36, want: domain.VerdictManualReview},
		{name: "score 0.74 below DenyAbove", score: 0.74, want: domain.VerdictManualReview},
		{name: "score exactly DenyAbove 0.75 is ManualReview", score: 0.75, want: domain.VerdictManualReview},
		{name: "score 0.76 above DenyAbove", score: 0.76, want: domain.VerdictDeny},
		{name: "score 1.0", score: 1.0, want: domain.VerdictDeny},
		{name: "negative score", score: -0.5, want: domain.VerdictApprove},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.VeredictFromScore(tt.score)
			if got != tt.want {
				t.Errorf("VeredictFromScore(%v) = %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}

func TestVeredictFromScoreConstants(t *testing.T) {
	if domain.ApproveBelow != 0.35 {
		t.Errorf("ApproveBelow = %v, want 0.35", domain.ApproveBelow)
	}
	if domain.DenyAbove != 0.75 {
		t.Errorf("DenyAbove = %v, want 0.75", domain.DenyAbove)
	}
}
