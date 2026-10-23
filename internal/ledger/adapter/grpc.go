package adapter

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/coigo/payment-processor-course/internal/ledger/domain"
	"github.com/coigo/payment-processor-course/internal/ledger/usecase"
	ledgerv1 "github.com/coigo/payment-processor-course/proto/gen/go/ledger/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	ledgerv1.UnimplementedLedgerServiceServer
	authorizer *usecase.Authorizer
}

func New(authorizer *usecase.Authorizer) *Server {
	return &Server{authorizer: authorizer}
}

func (s *Server) Authorize(ctx context.Context, req *ledgerv1.AuthorizeRequest) (*ledgerv1.AuthorizeResponse, error) {

	start := time.Now()

	amountCents, err := strconv.ParseInt(req.GetAmountCents(), 10, 64)
	if err != nil || amountCents <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid amount_cents")
	}

	charge := domain.Charge{
		IdempotencyKey:   req.GetIdempotencyKey(),
		AccountId:        req.GetAccountId(),
		CardFingerprint:  req.GetCardFingerprint(),
		AmountCents:      amountCents,
		Currency:         req.GetCurrency(),
		ClientIP:         req.GetClientIp(),
		MerchantCategory: req.GetMerchantCategory(),
	}

	result, err := s.authorizer.Authorize(ctx, charge)
	if err != nil {
		return nil, mapErr(err)
	}

	authorizeDuration.WithLabelValues().Observe(time.Since(start).Seconds())
	countDecision(result.Decision.Status, result.Replayed, charge.AmountCents)
	
	return &ledgerv1.AuthorizeResponse{
		PaymentId: result.PaymentId,
		Status:    statusToProto(result.Decision.Status),
		RiskScore: result.Decision.RiskScore,
		Reasons:   result.Decision.Reasons,
		Replayed:  result.Replayed,
	}, nil
}

func (s *Server) GetAccountHistory(ctx context.Context, req *ledgerv1.GetAccountHistoryRequest) (*ledgerv1.GetAccountHistoryResponse, error) {
	entries, err := s.authorizer.History(ctx, req.GetAccountId(), int(req.GetLimit()))
	if err != nil {
		return nil, status.Error(codes.Internal, "account history failed")
	}

	out := make([]*ledgerv1.HistoryEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, &ledgerv1.HistoryEntry{
			PaymentId:        strconv.FormatInt(e.PaymentId, 10),
			AmountCents:      e.AmountCents,
			Currency:         e.Currency,
			Status:           e.Status,
			MerchantCategory: e.MerchantCategory,
			OccurredAtUnixMs: e.OccuredAt.UnixMilli(),
		})
	}

	return &ledgerv1.GetAccountHistoryResponse{Entries: out}, nil
}

func (s *Server) ListReviews(ctx context.Context, req *ledgerv1.ListReviewsRequest) (*ledgerv1.ListReviewsResponse, error) {
	reviews, err := s.authorizer.Reviews(ctx, req.GetStatus(), int(req.GetLimit()))
	if err != nil {
		if strings.Contains(err.Error(), "unknown review status") {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		return nil, status.Error(codes.Internal, "list reviews failed")
	}

	out := make([]*ledgerv1.Review, 0, len(reviews))
	for _, r := range reviews {
		out = append(out, &ledgerv1.Review{
			PaymentId:        r.PaymentId,
			Status:           r.Status,
			Report:           r.Report,
			AmountCents:      r.AmountCents,
			Currency:         r.Currency,
			MerchantCategory: r.MerchantCategory,
			RiskScore:        r.Decision.RiskScore,
			CreatedAtUnixMs:  r.CreatedAt.UnixMilli(),
			UpdatedAtUnixMs:  r.UpdatedAt.UnixMilli(),
			Recommendation:   r.Recomendation,
			Decision:         r.Decision.Status,
			DecidedBy:        r.DecidedBy,
		})
	}

	return &ledgerv1.ListReviewsResponse{Reviews: out}, nil
}

func (s *Server) DecideReview(ctx context.Context, req *ledgerv1.DecideReviewRequest) (*ledgerv1.DecideReviewResponse, error) {
	result, err := s.authorizer.DecideReview(ctx,
		req.GetPaymentId(), req.GetDecision(), req.GetOperator())
	if err != nil {
		switch {
		case errors.Is(err, usecase.ErrInvalidDecision): return nil, status.Error(codes.InvalidArgument, err.Error())
		case errors.Is(err, usecase.ErrRevewNotFound): return nil, status.Error(codes.NotFound, err.Error())
		case errors.Is(err, usecase.ErrReviewAlreadyDecided): return nil, status.Error(codes.AlreadyExists, err.Error())
		default: return nil, status.Error(codes.Internal, "decision failed")
		}
	}

	countReviewDecision(result.Status, result.FollowedRecommendation)
	
	return &ledgerv1.DecideReviewResponse{
		PaymentId: result.PaymentId,
		Status: result.Status,
		FollowedRecommendation: result.FollowedRecommendation,
	}, nil
}

func (s *Server) GetRecentPayments(ctx context.Context, req *ledgerv1.GetRecentPaymentsRequest) (*ledgerv1.GetRecentPaymentsResponse, error) {
	payments, err := s.authorizer.Recent(ctx, int(req.GetLimit()))
	if err != nil {
		return nil, status.Error(codes.Internal, "recent payments failed")
	}

	out := make([]*ledgerv1.RecentPayment, 0, len(payments))
	for _, p := range payments {
		out = append(out, &ledgerv1.RecentPayment{
			PaymentId:        p.PaymentId,
			AccountId:        p.AccountId,
			AmountCents:      p.AmountCents,
			Currency:         p.Currency,
			Status:           p.Status,
			RiskScore:        p.RiskScore,
			Reasons:          p.Reasons,
			MerchantCategory: p.MerchantCategory,
			CreatedAtUnixMs:  p.CreatedAt.UnixMilli(),
		})
	}

	return &ledgerv1.GetRecentPaymentsResponse{Payments: out}, nil
}

func statusToProto(s string) ledgerv1.PaymentStatus {
	switch s {
		case domain.StatusApproved: return ledgerv1.PaymentStatus_PAYMENT_STATUS_APPROVED
		case domain.StatusDenied: return ledgerv1.PaymentStatus_PAYMENT_STATUS_DENIED
		case domain.StatusManualReview: return ledgerv1.PaymentStatus_PAYMENT_STATUS_MANUAL_REVIEW
		default: return ledgerv1.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}

func mapErr (err error) error {
	switch {
		case errors.Is(err, domain.ErrInvalidAmount),
			errors.Is(err, domain.ErrMissingAccount),
			errors.Is(err, domain.ErrMissingIdempotencyKey):
			return status.Error(codes.InvalidArgument, err.Error())
			
		case errors.Is(err, domain.ErrIdempotencyConflict):
			return status.Error(codes.AlreadyExists, err.Error())
			
		default:
			return status.Errorf(codes.Internal, "authorization failed")
	}
}