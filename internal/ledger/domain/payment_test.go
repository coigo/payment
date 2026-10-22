package domain_test

import (
	"errors"
	"testing"
	"time"

	"github.com/coigo/payment-processor-course/internal/ledger/domain"
)

func validCharge() domain.Charge {
	return domain.Charge{
		IdempotencyKey:   "idem-1",
		AccountId:        "acc-1",
		CardFingerprint:  "fp-1",
		AmountCents:      1299,
		Currency:         "usd",
		ClientIP:         "203.0.113.7",
		MerchantCategory: "grocery",
		OccurredAt:       time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC),
	}
}

func TestChargeValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*domain.Charge)
		wantErr error
	}{
		{"valid charge", func(*domain.Charge) {}, nil},
		{"missing idempotency key", func(c *domain.Charge) { c.IdempotencyKey = "" }, domain.ErrMissingIdempotencyKey},
		{"missing account id", func(c *domain.Charge) { c.AccountId = "" }, domain.ErrMissingAccount},
		{"zero amount", func(c *domain.Charge) { c.AmountCents = 0 }, domain.ErrInvalidAmount},
		{"negative amount", func(c *domain.Charge) { c.AmountCents = -1 }, domain.ErrInvalidAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := validCharge()
			tt.mutate(&c)

			err := c.Validate()

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate() error = %v, want nil", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("Validate() error = %v, want errors.Is(%v)", err, tt.wantErr)
			}
		})
	}
}

func TestEntriesFor(t *testing.T) {
	c := validCharge()

	entries := domain.EntriesFor(c)

	if len(entries) != 2 {
		t.Fatalf("EntriesFor() len = %d, want 2", len(entries))
	}

	debit := entries[0]
	if debit.Direction != domain.DirectionDebit {
		t.Errorf("EntriesFor()[0].Direction = %q, want %q", debit.Direction, domain.DirectionDebit)
	}
	if debit.AccountId != "acc-1" {
		t.Errorf("EntriesFor()[0].AccountId = %q, want %q", debit.AccountId, "acc-1")
	}
	if debit.AmountCents != 1299 {
		t.Errorf("EntriesFor()[0].AmountCents = %d, want %d", debit.AmountCents, 1299)
	}

	credit := entries[1]
	if credit.Direction != domain.DirectionCredit {
		t.Errorf("EntriesFor()[1].Direction = %q, want %q", credit.Direction, domain.DirectionCredit)
	}
	if credit.AccountId != domain.MerchantSettlementAccount {
		t.Errorf("EntriesFor()[1].AccountId = %q, want %q", credit.AccountId, domain.MerchantSettlementAccount)
	}
	if credit.AmountCents != 1299 {
		t.Errorf("EntriesFor()[1].AmountCents = %d, want %d", credit.AmountCents, 1299)
	}
}

func TestBalanced(t *testing.T) {
	tests := []struct {
		name    string
		entries []domain.Entry
		want    bool
	}{
		{
			name:    "entries from EntriesFor balance out",
			entries: domain.EntriesFor(validCharge()),
			want:    true,
		},
		{
			name: "debit equals credit",
			entries: []domain.Entry{
				{Direction: domain.DirectionDebit, AccountId: "acc-1", AmountCents: 500},
				{Direction: domain.DirectionCredit, AccountId: domain.MerchantSettlementAccount, AmountCents: 500},
			},
			want: true,
		},
		{
			name: "debit exceeds credit",
			entries: []domain.Entry{
				{Direction: domain.DirectionDebit, AccountId: "acc-1", AmountCents: 500},
				{Direction: domain.DirectionCredit, AccountId: domain.MerchantSettlementAccount, AmountCents: 400},
			},
			want: false,
		},
		{
			name: "credit exceeds debit",
			entries: []domain.Entry{
				{Direction: domain.DirectionDebit, AccountId: "acc-1", AmountCents: 100},
				{Direction: domain.DirectionCredit, AccountId: domain.MerchantSettlementAccount, AmountCents: 200},
			},
			want: false,
		},
		{
			name: "multiple entries sum to zero",
			entries: []domain.Entry{
				{Direction: domain.DirectionDebit, AccountId: "acc-1", AmountCents: 700},
				{Direction: domain.DirectionDebit, AccountId: "acc-2", AmountCents: 300},
				{Direction: domain.DirectionCredit, AccountId: domain.MerchantSettlementAccount, AmountCents: 1000},
			},
			want: true,
		},
		{name: "empty entry list", entries: []domain.Entry{}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.Balanced(tt.entries); got != tt.want {
				t.Errorf("Balanced(entries) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStatusesAreDistinct(t *testing.T) {
	if domain.StatusApproved != "approved" {
		t.Errorf("StatusApproved = %q, want %q", domain.StatusApproved, "approved")
	}
	if domain.StatusDenied != "denied" {
		t.Errorf("StatusDenied = %q, want %q", domain.StatusDenied, "denied")
	}
	if domain.StatusManualReview != "manual_review" {
		t.Errorf("StatusManualReview = %q, want %q", domain.StatusManualReview, "manual_review")
	}
	if len(map[string]bool{domain.StatusApproved: true, domain.StatusDenied: true, domain.StatusManualReview: true}) != 3 {
		t.Error("status constants must not collide")
	}
}
