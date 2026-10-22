package domain

import (
	"errors"
	"time"
)

const (
	StatusApproved     = "approved"
	StatusDenied       = "denied"
	StatusManualReview = "manual_review"
)

const (
	DirectionDebit  = "debit"
	DirectionCredit = "credit"
)

const MerchantSettlementAccount = "00000000-0000-0000-0000-000000000001"

var (
	ErrInvalidAmount         = errors.New("ledger: amount must be positive")
	ErrMissingAccount        = errors.New("ledger: account id is required")
	ErrMissingIdempotencyKey = errors.New("ledger: idempotency key is required")
	ErrIdempotencyConflict   = errors.New("ledger: idempotency key reused")
)

type Charge struct {
	IdempotencyKey   string
	AccountId        string
	CardFingerprint  string
	AmountCents      int64
	Currency         string
	ClientIP         string
	MerchantCategory string
	OccurredAt       time.Time
}

type Entry struct {
	Direction   string
	AccountId   string
	AmountCents int64
}

type Decision struct {
	Status    string
	RiskScore float64
	Reasons   []string
	Degraded  bool
}

type Result struct {
	PaymentId string
	Decision  Decision
	Replayed  bool
}

func EntriesFor(c Charge) []Entry {
	return []Entry{
		{AccountId: c.AccountId, Direction: DirectionDebit, AmountCents: c.AmountCents},
		{AccountId: MerchantSettlementAccount, Direction: DirectionCredit, AmountCents: c.AmountCents},
	}
}

func Balanced(entries []Entry) bool {
	var sum int64
	for _, entry := range entries {
		if entry.Direction == DirectionDebit {
			sum += entry.AmountCents
		} else {
			sum -= entry.AmountCents
		}
	}
	return sum == 0
}

// Validate checks that a charge carries an idempotency key, an account
// and a positive amount, returning the first violation as a sentinel error.
func (c *Charge) Validate() error {
	if c.IdempotencyKey == "" {
		return ErrMissingIdempotencyKey
	}
	if c.AccountId == "" {
		return ErrMissingAccount
	}
	if c.AmountCents <= 0 {
		return ErrInvalidAmount
	}

	return nil

}
