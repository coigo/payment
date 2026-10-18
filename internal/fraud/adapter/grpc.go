package adapter

import (
	"context"
	"log/slog"
	"time"

	"github.com/coigo/payment-processor-course/internal/fraud/domain"
	"github.com/coigo/payment-processor-course/internal/fraud/usecases"
	fraudv1 "github.com/coigo/payment-processor-course/proto/gen/go/fraud/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Server struct {
	fraudv1.UnimplementedFraudServiceServer

	pipeline 	*usecases.Pipeline
	logger 		*slog.Logger
}

func NewServer(pipeline *usecases.Pipeline, logger *slog.Logger) *Server {
	return &Server{
		pipeline: pipeline,
		logger: logger,
	}
}

func (s *Server) Score (ctx context.Context, req *fraudv1.ScoreRequest) (*fraudv1.ScoreResponse, error) {
	attempt := domain.Attempt {
		PaymentId: req.GetPaymentId(),
		AccountId: req.GetAccountId(),
		CardFingerPrint: req.GetCardFingerprint(),
		AmountCents: req.GetAmountCents(),
		ClientIP: req.GetClientIp(),
		Currency: req.GetCurrency(),
		OutcurredAt: time.UnixMilli(req.GetOccurredAtUnixMs()).UTC(),
		MerchantCategory: req.GetMerchantCategory(),
		
	}
	start := time.Now()
	decision, trace, err := s.pipeline.Score(ctx, attempt)
	if err != nil {
		s.logger.Error("failed to score attempt", 
			slog.String("payment_id", attempt.PaymentId),
			slog.String("error", err.Error()))
		return nil, status.Error(codes.Internal, "scoring failed")
	}

	scoreDuration.WithLabelValues().Observe(time.Since(start).Seconds())
	countVerdict(string(decision.Verdict), decision.Degraded)
	countReasons(decision.Reasons)
	observeRiskScore(decision.Riskscore)
	for _, hop := range trace {
		observeStage(string(hop.From), hop.Duration.Seconds())
	}
	
	s.logger.Info("attempt scored", 
		slog.String("payment_id", attempt.PaymentId),
		slog.String("verdict", string(decision.Verdict)),
		slog.Float64("risk_score", decision.Riskscore),
		slog.Int("pipeline_hops", len(trace)),
		slog.Duration("took", time.Since(start)),
	)
	
	return &fraudv1.ScoreResponse{
		RiskScore: decision.Riskscore,
		Reasons: decision.Reasons,
		Degraded: decision.Degraded,
	}, nil
}

func verdictToProto (v domain.Verdict) fraudv1.Verdict {
	switch v {
		case domain.VerdictApprove:
			return fraudv1.Verdict_VERDICT_APPROVE
		case domain.VerdictDeny:
			return fraudv1.Verdict_VERDICT_DENY
		case domain.VerdictManualReview:
			return fraudv1.Verdict_VERDICT_MANUAL_REVIEW
		default:
			return fraudv1.Verdict_VERDICT_UNSPECIFIED
	}
}