package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/coigo/payment-processor-course/internal/ledger/domain"
	"github.com/google/uuid"
)

type Authorizer struct {
	repo 		Repository
	fraud 		FraudScore
	processor 	PaymentProcessor
	logger 		*slog.Logger

	fraudTimeout time.Duration
}

func NewAuthorizer(repo Repository, fraud FraudScore, processor PaymentProcessor, logger *slog.Logger) *Authorizer {
	return &Authorizer{
		repo: repo,
		fraud: fraud,
		processor: processor,
		logger: logger,
		fraudTimeout: 100 * time.Millisecond,
	}
}

type FraudScore interface {
	Score(ctx context.Context, PaymentId string, c domain.Charge) (domain.Decision, error)
}

type PaymentProcessor interface {
	Charge (ctx context.Context, paymentId string, c domain.Charge) error
}

type Repository interface {
	FindIdempotency(ctx context.Context, key, requestHash string) (
		result domain.Result, found bool, conflict bool, err error)
	PersistDecision(ctx context.Context, cmd PersistCommand) error
	AccountHistory(ctx context.Context, accountId string, lint int) ([]HistoryEntry, error)
	ListReviews(ctx context.Context, status string, lint int) ([]ReviewEntry, error)
	RecentPayments(ctx context.Context, limit int) ([]PaymentEntry, error)
	
	
}

type HistoryEntry struct {
	PaymentId 			int64
	AccountId 			int64
	AmountCents 		int64
	Currency 			string
	CreatedAt 			int64
	MerchantCategory	string
	Status 				string
	OccuredAt			time.Time
}

type PersistCommand struct {
	PaymentId 		string
	Charge 			domain.Charge
	Decision 		domain.Decision
	Entries 		[]domain.Entry
	ReqyestHash 	string
}

type PaymentEntry struct {
	PaymentId 			string
	AccountId 			string
	AmountCents 		int64
	Currency 			string
	Status 				string
	RiskScore 			float64
	Reasons 			[]string
	Charge 				domain.Charge
	MerchantCategory 	string
	CreatedAt 			time.Time
}

type ReviewEntry struct {
	PaymentId 			string
	AccountId 			string
	AmountCents 		int64
	Currency 			string
	Status 				string
	Report				string
	Recomendation 		string
	Decision			domain.Decision
	DecidedBy			string
	MerchantCategory 	string
	CreatedAt 			time.Time
	UpdatedAt			time.Time
}

func (a *Authorizer) History (ctx context.Context, AccountId string, limit int) ([]HistoryEntry, error ) {
	return a.repo.AccountHistory(ctx, AccountId, clampLimit(limit))
}

func (a *Authorizer) Reviews (ctx context.Context, Status string, limit int) ([]ReviewEntry, error ) {
	switch	Status{
		case "", "pending", "investigating", "resolved", "closed": 
		default:
			return nil, fmt.Errorf("ledger: unknown review status: %q", Status)
	}
	
	return a.repo.ListReviews(ctx, Status, clampLimit(limit))
}

func (a *Authorizer) Recent (ctx context.Context, limit int) ([]PaymentEntry, error ) {
	return a.repo.RecentPayments(ctx, clampLimit(limit))
}

func clampLimit (limit int) int {
	if limit < 0 || limit > 100 {
		return 20
	}
	return limit
}

func (a *Authorizer) Authorize(ctx context.Context, charge domain.Charge) (domain.Result, error) {
	if err := charge.Validate(); err != nil {
		return domain.Result{}, err
	}
	if charge.OccurredAt.IsZero() {
		charge.OccurredAt = time.Now().UTC()
	} 
	requestHash := hashCharge(charge)

	stored, found, conflict, err := a.repo.FindIdempotency(ctx, charge.IdempotencyKey, requestHash)
	if err != nil {
		return domain.Result{}, fmt.Errorf("find idempotency: %w", err)
	}
	if found {
		stored.Replayed = true
		return stored, nil
	}
	if conflict {
		return domain.Result{}, domain.ErrIdempotencyConflict
	}

	paymentId := uuid.NewString()
	decision := a.scoreWithBudget(ctx,  paymentId, charge)

	var entries []domain.Entry
	if decision.Status == domain.StatusApproved {
		if err := a.processor.Charge(ctx, paymentId, charge); err != nil {
			return domain.Result{}, fmt.Errorf("payment processor: %w", err)
		}
		entries = domain.EntriesFor(charge)
		if !domain.Balanced(entries) {
			return domain.Result{}, fmt.Errorf("payment processor: balance mismatch")
		}
	}

	cmd := PersistCommand{
		PaymentId: paymentId,
		Charge: charge,
		Decision: decision,
		Entries: entries,
		ReqyestHash: requestHash,
		
	}

	if err := a.repo.PersistDecision(ctx, cmd); err != nil {
		return domain.Result{}, fmt.Errorf("persist decision: %w", err )
	}
	return domain.Result{
		PaymentId: paymentId,
		Decision: decision,
		
	}, nil
}

func hashCharge (c domain.Charge) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s", c.AccountId, c.CardFingerprint, c.AmountCents, c.Currency )
	return hex.EncodeToString(h.Sum(nil))
}

func (a *Authorizer) scoreWithBudget (ctx context.Context, paymentId string, c domain.Charge) (domain.Decision) {
	scoreCtx, cancel := context.WithTimeout(ctx, a.fraudTimeout)
	defer cancel()
	
	decision, err := a.fraud.Score(scoreCtx, paymentId, c)
	if err != nil {
		a.logger.Warn("fraud scorer unavaliable, routing to manual review", 
			slog.String("payment_id", paymentId), 
			slog.String("error", err.Error()))
		
		return domain.Decision{
			Status: domain.StatusManualReview,
			Reasons: []string{"fraud_service_unavailable"},
			Degraded: true,
		}
	}
	return decision
}