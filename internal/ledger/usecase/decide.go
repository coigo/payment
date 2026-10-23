package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/coigo/payment-processor-course/internal/ledger/domain"
)

var (
	ErrRevewNotFound		= errors.New("ledger: review not found")
	ErrReviewAlreadyDecided = errors.New("ledger: review already decided")
	ErrInvalidDecision		= errors.New("ledger: decision must be approved or denied")
	
)

type ReviewCase struct {
	PaymentId 			string
	ReviewStatus		string
	Recommendation		string
	AccountId 			string
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
	ReviewCase(ctx context.Context, paymentId string) (c ReviewCase, found bool, err error)
	ApplyDecision(ctx context.Context, cmd DecisionCommand) (applied bool, err error)
}

func (a *Authorizer)DecideReview (ctx context.Context, paymentId, decision, operator string) (DecisionResult, error) {
	if decision != domain.StatusApproved && decision != domain.StatusDenied {
		return DecisionResult{}, ErrInvalidDecision
	}
	if operator == "" {
		operator = "unknown"
	}

	c, found, err := a.repo.ReviewCase(ctx, paymentId)
	if err != nil { return DecisionResult{}, fmt.Errorf("load review: %w", err)}
	if !found { return DecisionResult{}, ErrRevewNotFound}
	if c.ReviewStatus == "closed" { return DecisionResult{}, ErrReviewAlreadyDecided}

	charge := domain.Charge{
		AccountId: c.AccountId,
		Currency: c.Currency,
		MerchantCategory: c.MerchantCategory,
		ClientIP: c.ClientIp,
		CardFingerprint: c.CardFingerprint,
	}

	var entries []domain.Entry
	if decision == domain.StatusApproved {
		if err := a.processor.Charge(ctx, paymentId, charge); err != nil {
			return DecisionResult{}, fmt.Errorf("payment processor: %w", err)
		}
		entries = domain.EntriesFor(charge)
		if ! domain.Balanced(entries) {
			return DecisionResult{}, fmt.Errorf("payment processor: balance mismatch")
		}
	}
	applied, err := a.repo.ApplyDecision(ctx,  DecisionCommand{
		PaymentId: paymentId,
		Decision: decision,
		Operator: operator,
		Entries: entries,
		DecidedAt: time.Now().UTC(),
	})
	
	if err != nil {
		return DecisionResult{}, fmt.Errorf("apply decision: %w", err) 
	}
	if !applied {
		return DecisionResult{}, ErrReviewAlreadyDecided 
		
	}
	a.logger.Info("review decided", 
		slog.String("payment_id", paymentId),
		slog.String("decision", decision),
		slog.String("operator", operator),
		slog.String("recommendation", c.Recommendation),
	
	)
	return DecisionResult{
		PaymentId: paymentId,
		Status: decision,
		FollowedRecommendation: decision == recommendationToStatus(c.Recommendation),
	}, nil
	
}

func recommendationToStatus(r string) (string) {
	switch r{
		case "approve":
			return domain.StatusApproved
		case "deny":
			return domain.StatusDenied 		
		default:
			return ""
	}
}