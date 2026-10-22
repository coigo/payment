package adapter

import (
	"errors"

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
			errors.Is(err, domain.ErrMissingAccount):
			return status.Error(codes.InvalidArgument, err.Error())
			
		case errors.Is(err, domain.ErrIdempotencyConflict),
			errors.Is(err, domain.ErrMissingAccount):
			return status.Error(codes.AlreadyExists, err.Error())
			
		default:
			return status.Errorf(codes.Internal, "authorization failed")
	}
}