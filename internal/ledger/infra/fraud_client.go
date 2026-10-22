package infra

import (
	"context"
	"fmt"

	"github.com/coigo/payment-processor-course/internal/ledger/domain"
	fraudv1 "github.com/coigo/payment-processor-course/proto/gen/go/fraud/v1"
)

type FraudClient struct {
	client fraudv1.FraudServiceClient
}

func NewFraudClient (client fraudv1.FraudServiceClient) *FraudClient {
	return &FraudClient{
		client: client,
	}
}

func (f FraudClient) Score (ctx context.Context, paymentId string, c domain.Charge) (domain.Decision, error) {
	resp, err := f.client.Score(ctx, &fraudv1.ScoreRequest{
		PaymentId: paymentId,
		AccountId: c.AccountId,
		CardFingerprint: c.CardFingerprint,
		AmountCents: c.AmountCents,
		Currency: c.Currency,
		ClientIp: c.ClientIP,
		MerchantCategory: c.MerchantCategory,
		OccurredAtUnixMs: c.OccurredAt.UnixMilli(),
	})
	if err != nil {
		return domain.Decision{}, fmt.Errorf("fraud grpc: failed to score payment: %w", err)
	}
	status, err := verdictToStatus(resp.Verdict)
	if err != nil {
		return domain.Decision{}, fmt.Errorf("fraud grpc: failed to convert verdict to status %w", err)
	}
	return domain.Decision{
		Status: status,
		RiskScore: resp.GetRiskScore(),
		Reasons: resp.GetReasons(),
		Degraded: resp.GetDegraded(),
	}, nil
}

func verdictToStatus (v fraudv1.Verdict) (string, error) {
	switch v {
		case fraudv1.Verdict_VERDICT_APPROVE:
			return domain.StatusApproved, nil
		case fraudv1.Verdict_VERDICT_DENY:
			return domain.StatusDenied, nil
		case fraudv1.Verdict_VERDICT_MANUAL_REVIEW:
			return domain.StatusManualReview, nil
		default:
			return "", fmt.Errorf("fraud grpc: unknown verdict: %v", v)
	}
}