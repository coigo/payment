package domain_test

import (
	"testing"

	"github.com/coigo/payment-processor-course/internal/fraud/domain"
)

func TestApplyFastRules(t *testing.T) {
	tests := []struct {
		name string
		attempt domain.Attempt
		wantVerdict domain.Verdict
		wantReason  string
		wantFinal   bool
	}{
		{
			name: "blocklisted card fp_stolen_card_001",
			attempt: domain.Attempt{
				CardFingerPrint: "fp_stolen_card_001",
				AmountCents:     100,
				VelocityLastMin: 1,
			},
			wantVerdict: domain.VerdictDeny,
			wantReason:  "card_blocklisted",
			wantFinal:   true,
		},
		{
			name: "blocklisted card fp_stolen_card_002",
			attempt: domain.Attempt{
				CardFingerPrint: "fp_stolen_card_002",
				AmountCents:     100,
				VelocityLastMin: 1,
			},
			wantVerdict: domain.VerdictDeny,
			wantReason:  "card_blocklisted",
			wantFinal:   true,
		},
		{
			name: "amount exactly at MaxAmountCents 5000000",
			attempt: domain.Attempt{
				CardFingerPrint: "normal_fp",
				AmountCents:      5000000,
				VelocityLastMin:  1,
			},
			wantVerdict: domain.VerdictApprove,
			wantReason:  "fast_rules_passed",
			wantFinal:   false,
		},
		{
			name: "amount just above MaxAmountCents 5000001",
			attempt: domain.Attempt{
				CardFingerPrint: "normal_fp",
				AmountCents:      5000001,
				VelocityLastMin:  1,
			},
			wantVerdict: domain.VerdictDeny,
			wantReason:  "amount_above_hard_limit",
			wantFinal:   true,
		},
		{
			name: "velocity exactly at limit 10",
			attempt: domain.Attempt{
				CardFingerPrint: "normal_fp",
				AmountCents:      100,
				VelocityLastMin:  10,
			},
			wantVerdict: domain.VerdictApprove,
			wantReason:  "fast_rules_passed",
			wantFinal:   false,
		},
		{
			name: "velocity just above limit 11",
			attempt: domain.Attempt{
				CardFingerPrint: "normal_fp",
				AmountCents:      100,
				VelocityLastMin:  11,
			},
			wantVerdict: domain.VerdictDeny,
			wantReason:  "velocity_limit_exceeded",
			wantFinal:   true,
		},
		{
			name: "happy path",
			attempt: domain.Attempt{
				CardFingerPrint: "normal_fp",
				AmountCents:      100000,
				VelocityLastMin:  2,
			},
			wantVerdict: domain.VerdictApprove,
			wantReason:  "fast_rules_passed",
			wantFinal:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.ApplyFastRules(tt.attempt)
			if got.Verdict != tt.wantVerdict {
				t.Errorf("Verdict = %v, want %v", got.Verdict, tt.wantVerdict)
			}
			if got.Reason != tt.wantReason {
				t.Errorf("Reason = %v, want %v", got.Reason, tt.wantReason)
			}
			if got.Final != tt.wantFinal {
				t.Errorf("Final = %v, want %v", got.Final, tt.wantFinal)
			}
		})
	}
}

func TestApplyFastRulesConstants(t *testing.T) {
	if domain.MaxAmountCents != 5000000 {
		t.Errorf("MaxAmountCents = %d, want 5000000", domain.MaxAmountCents)
	}
	if domain.VelocityPerMinLimit != 10 {
		t.Errorf("VelocityPerMinLimit = %d, want 10", domain.VelocityPerMinLimit)
	}
}
