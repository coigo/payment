package usecase_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/coigo/payment-processor-course/internal/ledger/domain"
	"github.com/coigo/payment-processor-course/internal/ledger/usecase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func chargeHash(c domain.Charge) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s|%s|%d|%s", c.AccountId, c.CardFingerprint, c.AmountCents, c.Currency)
	return hex.EncodeToString(h.Sum(nil))
}

type fakeRepo struct {
	findResult domain.Result
	found      bool
	conflict   bool
	findErr    error
	persistErr error

	findCalls    int
	persistCalls int
	lastPersist  usecase.PersistCommand

	historyLimit int
	reviewsLimit int
	recentLimit  int
	reviewsStatus string
}

func (f *fakeRepo) FindIdempotency(ctx context.Context, key, requestHash string) (domain.Result, bool, bool, error) {
	f.findCalls++
	return f.findResult, f.found, f.conflict, f.findErr
}

func (f *fakeRepo) PersistDecision(ctx context.Context, cmd usecase.PersistCommand) error {
	f.persistCalls++
	f.lastPersist = cmd
	return f.persistErr
}

func (f *fakeRepo) AccountHistory(ctx context.Context, accountId string, limit int) ([]usecase.HistoryEntry, error) {
	f.historyLimit = limit
	return []usecase.HistoryEntry{{AccountId: 1, AmountCents: 1299}}, nil
}

func (f *fakeRepo) ListReviews(ctx context.Context, status string, limit int) ([]usecase.ReviewEntry, error) {
	f.reviewsStatus = status
	f.reviewsLimit = limit
	return []usecase.ReviewEntry{{PaymentId: "pay-1", Status: status}}, nil
}

func (f *fakeRepo) RecentPayments(ctx context.Context, limit int) ([]usecase.PaymentEntry, error) {
	f.recentLimit = limit
	return []usecase.PaymentEntry{{PaymentId: "pay-1"}}, nil
}

func (f *fakeRepo) ReviewCase(ctx context.Context, paymentId string) (usecase.ReviewCase, bool, error) {
	return usecase.ReviewCase{}, false, nil
}

func (f *fakeRepo) ApplyDecision(ctx context.Context, cmd usecase.DecisionCommand) (bool, error) {
	return true, nil
}

type fakeFraud struct {
	decision domain.Decision
	err      error

	calls         int
	lastPaymentId string
	lastCharge    domain.Charge
}

func (f *fakeFraud) Score(ctx context.Context, paymentId string, c domain.Charge) (domain.Decision, error) {
	f.calls++
	f.lastPaymentId = paymentId
	f.lastCharge = c
	return f.decision, f.err
}

type fakeProcessor struct {
	err error

	calls         int
	lastPaymentId string
	lastCharge    domain.Charge
}

func (f *fakeProcessor) Charge(ctx context.Context, paymentId string, c domain.Charge) error {
	f.calls++
	f.lastPaymentId = paymentId
	f.lastCharge = c
	return f.err
}

func newAuthorizer(t *testing.T, repo *fakeRepo, fraud *fakeFraud, processor *fakeProcessor) *usecase.Authorizer {
	t.Helper()
	return usecase.NewAuthorizer(repo, fraud, processor, slog.New(slog.DiscardHandler))
}

func TestAuthorizeInvalidCharge(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*domain.Charge)
		wantErr error
	}{
		{"missing idempotency key", func(c *domain.Charge) { c.IdempotencyKey = "" }, domain.ErrMissingIdempotencyKey},
		{"missing account id", func(c *domain.Charge) { c.AccountId = "" }, domain.ErrMissingAccount},
		{"zero amount", func(c *domain.Charge) { c.AmountCents = 0 }, domain.ErrInvalidAmount},
		{"negative amount", func(c *domain.Charge) { c.AmountCents = -1 }, domain.ErrInvalidAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			fraud := &fakeFraud{}
			proc := &fakeProcessor{}
			auth := newAuthorizer(t, repo, fraud, proc)
			c := validCharge()
			tt.mutate(&c)

			result, err := auth.Authorize(context.Background(), c)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, domain.Result{}, result)
			assert.Zero(t, repo.findCalls)
			assert.Zero(t, fraud.calls)
			assert.Zero(t, proc.calls)
		})
	}
}

func TestAuthorizeReplayReturnsStoredResult(t *testing.T) {
	repo := &fakeRepo{
		found: true,
		findResult: domain.Result{
			PaymentId: "pay-stored",
			Decision:  domain.Decision{Status: domain.StatusApproved},
		},
	}
	fraud := &fakeFraud{}
	proc := &fakeProcessor{}
	auth := newAuthorizer(t, repo, fraud, proc)

	result, err := auth.Authorize(context.Background(), validCharge())

	require.NoError(t, err)
	assert.Equal(t, "pay-stored", result.PaymentId)
	assert.True(t, result.Replayed)
	assert.Equal(t, 1, repo.findCalls)
	assert.Zero(t, fraud.calls)
	assert.Zero(t, proc.calls)
	assert.Zero(t, repo.persistCalls)
}

func TestAuthorizeIdempotencyConflict(t *testing.T) {
	repo := &fakeRepo{conflict: true}
	fraud := &fakeFraud{}
	proc := &fakeProcessor{}
	auth := newAuthorizer(t, repo, fraud, proc)

	result, err := auth.Authorize(context.Background(), validCharge())

	require.ErrorIs(t, err, domain.ErrIdempotencyConflict)
	assert.Equal(t, domain.Result{}, result)
	assert.Equal(t, 1, repo.findCalls)
	assert.Zero(t, fraud.calls)
	assert.Zero(t, proc.calls)
}

func TestAuthorizeFindIdempotencyError(t *testing.T) {
	repo := &fakeRepo{findErr: errors.New("db down")}
	fraud := &fakeFraud{}
	proc := &fakeProcessor{}
	auth := newAuthorizer(t, repo, fraud, proc)

	result, err := auth.Authorize(context.Background(), validCharge())

	require.ErrorContains(t, err, "find idempotency")
	require.ErrorContains(t, err, "db down")
	assert.Equal(t, domain.Result{}, result)
	assert.Zero(t, fraud.calls)
	assert.Zero(t, proc.calls)
}

func TestAuthorizeApproved(t *testing.T) {
	c := validCharge()
	fraud := &fakeFraud{decision: domain.Decision{
		Status:  domain.StatusApproved,
		RiskScore: 0.1,
	}}
	proc := &fakeProcessor{}
	repo := &fakeRepo{}
	auth := newAuthorizer(t, repo, fraud, proc)

	result, err := auth.Authorize(context.Background(), c)

	require.NoError(t, err)
	assert.Equal(t, domain.StatusApproved, result.Decision.Status)
	assert.NotEmpty(t, result.PaymentId)

	require.Equal(t, 1, proc.calls)
	assert.Equal(t, result.PaymentId, proc.lastPaymentId)
	assert.Equal(t, c, proc.lastCharge)

	require.Equal(t, 1, fraud.calls)
	assert.Equal(t, c, fraud.lastCharge)

	require.Equal(t, 1, repo.persistCalls)
	assert.Equal(t, result.PaymentId, repo.lastPersist.PaymentId)
	assert.Equal(t, chargeHash(c), repo.lastPersist.ReqyestHash)
	assert.Equal(t, domain.EntriesFor(c), repo.lastPersist.Entries)
	assert.True(t, domain.Balanced(repo.lastPersist.Entries))
}

func TestAuthorizeProcessorFails(t *testing.T) {
	fraud := &fakeFraud{decision: domain.Decision{Status: domain.StatusApproved}}
	proc := &fakeProcessor{err: errors.New("psp timeout")}
	repo := &fakeRepo{}
	auth := newAuthorizer(t, repo, fraud, proc)

	result, err := auth.Authorize(context.Background(), validCharge())

	require.ErrorContains(t, err, "payment processor")
	require.ErrorContains(t, err, "psp timeout")
	assert.Equal(t, domain.Result{}, result)
	assert.Equal(t, 1, proc.calls)
	assert.Zero(t, repo.persistCalls)
}

func TestAuthorizeNonApprovedSkipsProcessor(t *testing.T) {
	tests := []struct {
		name       string
		decision   domain.Decision
		processFra bool
	}{
		{
			name:     "denied",
			decision: domain.Decision{Status: domain.StatusDenied, RiskScore: 0.9},
		},
		{
			name:     "manual review",
			decision: domain.Decision{Status: domain.StatusManualReview},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fraud := &fakeFraud{decision: tt.decision}
			proc := &fakeProcessor{}
			repo := &fakeRepo{}
			auth := newAuthorizer(t, repo, fraud, proc)

			result, err := auth.Authorize(context.Background(), validCharge())

			require.NoError(t, err)
			assert.Equal(t, tt.decision.Status, result.Decision.Status)
			assert.NotEmpty(t, result.PaymentId)
			assert.Zero(t, proc.calls)
			assert.Equal(t, 1, repo.persistCalls)
			assert.Empty(t, repo.lastPersist.Entries)
		})
	}
}

func TestAuthorizePersistDecisionFails(t *testing.T) {
	fraud := &fakeFraud{decision: domain.Decision{Status: domain.StatusApproved}}
	proc := &fakeProcessor{}
	repo := &fakeRepo{persistErr: errors.New("write conflict")}
	auth := newAuthorizer(t, repo, fraud, proc)

	result, err := auth.Authorize(context.Background(), validCharge())

	require.ErrorContains(t, err, "persist decision")
	require.ErrorContains(t, err, "write conflict")
	assert.Equal(t, domain.Result{}, result)
	assert.Equal(t, 1, repo.persistCalls)
}

func TestAuthorizeFillsZeroOccurredAt(t *testing.T) {
	c := validCharge()
	c.OccurredAt = time.Time{}
	fraud := &fakeFraud{decision: domain.Decision{Status: domain.StatusDenied}}
	proc := &fakeProcessor{}
	repo := &fakeRepo{}
	auth := newAuthorizer(t, repo, fraud, proc)

	_, err := auth.Authorize(context.Background(), c)

	require.NoError(t, err)
	stored := repo.lastPersist.Charge.OccurredAt
	assert.False(t, stored.IsZero())
	assert.WithinDuration(t, time.Now().UTC(), stored, time.Second)
	assert.Equal(t, time.UTC.String(), stored.Location().String())
}

func TestAuthorizeFraudUnavailableFallsBackToManualReview(t *testing.T) {
	fraud := &fakeFraud{err: errors.New("fraud service down")}
	proc := &fakeProcessor{}
	repo := &fakeRepo{}
	auth := newAuthorizer(t, repo, fraud, proc)

	result, err := auth.Authorize(context.Background(), validCharge())

	require.NoError(t, err)
	assert.Equal(t, domain.StatusManualReview, result.Decision.Status)
	assert.True(t, result.Decision.Degraded)
	assert.Equal(t, []string{"fraud_service_unavailable"}, result.Decision.Reasons)
	assert.Zero(t, proc.calls)
	assert.Equal(t, 1, repo.persistCalls)
	assert.Equal(t, domain.StatusManualReview, repo.lastPersist.Decision.Status)
}

func TestHistoryDelegatesWithClampedLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		wantUsed int
	}{
		{"within range passes through", 5, 5},
		{"zero passes through", 0, 0},
		{"negative clamps to 20", -1, 20},
		{"above 100 clamps to 20", 101, 20},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			auth := newAuthorizer(t, repo, nil, nil)

			entries, err := auth.History(context.Background(), "acc-1", tt.limit)

			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, int64(1299), entries[0].AmountCents)
			assert.Equal(t, tt.wantUsed, repo.historyLimit)
		})
	}
}

func TestReviews(t *testing.T) {
	tests := []struct {
		name        string
		status      string
		wantStatus  string
		wantErr     bool
	}{
		{"empty status", "", "", false},
		{"pending", "pending", "pending", false},
		{"resolved", "resolved", "resolved", false},
		{"closed", "closed", "closed", false},
		{"unknown status", "weird", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			auth := newAuthorizer(t, repo, nil, nil)

			entries, err := auth.Reviews(context.Background(), tt.status, 10)

			if tt.wantErr {
				require.ErrorContains(t, err, "unknown review status")
				assert.Empty(t, entries)
				assert.Empty(t, repo.reviewsStatus)
				return
			}
			require.NoError(t, err)
			require.Len(t, entries, 1)
			assert.Equal(t, tt.wantStatus, entries[0].Status)
			assert.Equal(t, tt.wantStatus, repo.reviewsStatus)
			assert.Equal(t, 10, repo.reviewsLimit)
		})
	}
}

func TestReviewsClampsLimit(t *testing.T) {
	tests := []struct {
		name     string
		limit    int
		wantUsed int
	}{
		{"negative clamps to 20", -5, 20},
		{"above 100 clamps to 20", 500, 20},
		{"within range passes through", 7, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeRepo{}
			auth := newAuthorizer(t, repo, nil, nil)

			_, err := auth.Reviews(context.Background(), "pending", tt.limit)

			require.NoError(t, err)
			assert.Equal(t, tt.wantUsed, repo.reviewsLimit)
		})
	}
}

func TestRecentDelegatesWithClampedLimit(t *testing.T) {
	repo := &fakeRepo{}
	auth := newAuthorizer(t, repo, nil, nil)

	entries, err := auth.Recent(context.Background(), 3)

	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "pay-1", entries[0].PaymentId)
	assert.Equal(t, 3, repo.recentLimit)
}
