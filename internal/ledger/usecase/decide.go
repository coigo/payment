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

type ReviewCase struct {
	PaymentId 			string
	ReviewStatus		string
	Recommendation		string
	AccountId 			int64
	CardFingerprint		string
	AmountCents 		int64
	Currency 			string
	MerchantCategory	string
	ClientIp			string
}

type DecisionCommand struct {
	PaymentId 			string
	Decision 			string
	Operator 			string
	Entries				[]domain.Entry
	DecidedAt			time.Time
}

type DecisionResult struct {
	PaymentId				string
	Status					string
	FollowedRecommendation	bool
}

type DecisionRepository interface {
	ReviewCase(ctx context.Context) (c ReviewCase, found bool, err error)
	ApplyDecision(ctx context.Context, cmd DecisionCommand) (applied bool, err error)
}