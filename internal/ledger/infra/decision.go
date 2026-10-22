package infra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/coigo/payment-processor-course/internal/ledger/usecase"
	"github.com/coigo/payment-processor-course/internal/plataform/kafka"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) ReviewCase(ctx context.Context, paymentID string) (usecase.ReviewCase, bool, error) {
	var c usecase.ReviewCase
	err := r.pool.QueryRow(ctx,
		`SELECT rv.payment_id, rv.status, rv.recommendation,
			p.account_id, p.card_fingerprint, p.amount_cents,
			p.currency, p.merchant_category, p.client_ip
		FROM reviews rv
		JOIN payments p ON p.id = rv.payment_id
		WHERE rv.payment_id = $1`, paymentID).
		Scan(&c.PaymentId, &c.ReviewStatus, &c.Recommendation,
			&c.AccountId, &c.CardFingerprint, &c.AmountCents,
			&c.Currency, &c.MerchantCategory, &c.ClientIp)
	
	if errors.Is(err, pgx.ErrNoRows) { return usecase.ReviewCase{}, false, nil }
	if err != nil { return usecase.ReviewCase{}, false, fmt.Errorf("query review case: %w", err) }
	return c, true, nil
}

func (r *Repository) ApplyDecision(ctx context.Context, cmd usecase.DecisionCommand) (bool, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	tag, err := tx.Exec(ctx,
		`UPDATE reviews
		SET status = 'closed', decision = $2, decided_by = $3,
			decided_at = $4, updated_at = now()
		WHERE payment_id = $1 AND status <> 'closed'`,
		cmd.PaymentId, cmd.Decision, cmd.Operator, cmd.DecidedAt)
	if err != nil {
		return false, fmt.Errorf("update review case: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return false, nil
	}

	_, err = tx.Exec(ctx,
		`UPDATE payments SET status = $2 WHERE id = $1 AND status = 'manual_review'`,
		cmd.PaymentId, cmd.Decision)
	if err != nil {
		return false, fmt.Errorf("update payment: %w", err)
	}

	for _, e := range cmd.Entries {
		_, err = tx.Exec(ctx,
			`INSERT INTO ledger_entries (payment_id, account_id, direction, amount_cents)
			VALUES ($1, $2, $3, $4)`,
			cmd.PaymentId, e.AccountId, e.Direction, e.AmountCents)
		if err != nil {
			return false, fmt.Errorf("insert ledger entry: %w", err)
		}
	}

	paymentEvent, err := r.decisionPaymentEvent(ctx, tx, cmd)
	if err != nil {
		return false, fmt.Errorf("generate payment event: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO outbox (topic, key, payload) VALUES ($1, $2, $3)`,
		kafka.TopicPaymentEvents, cmd.PaymentId, paymentEvent)
	if err != nil {
		return false, fmt.Errorf("insert payment event: %w", err)
	}

	step, err := json.Marshal(map[string]any{
		"payment_id": cmd.PaymentId,
		"step":       "human_decision",
		"detail":     fmt.Sprintf("%s by %s", cmd.Decision, cmd.Operator),
		"at":         cmd.DecidedAt,
	})
	if err != nil {
		return false, fmt.Errorf("marshal decision step: %w", err)
	}

	if _, err = tx.Exec(ctx,
		`INSERT INTO outbox (topic, key, payload) VALUES ($1, $2, $3)`,
		kafka.TopicReviewEvents, cmd.PaymentId, step); err != nil {
		return false, fmt.Errorf("insert payment step: %w", err)
	}

	return true, tx.Commit(ctx)
}

func (r *Repository) decisionPaymentEvent(ctx context.Context, tx pgx.Tx, cmd usecase.DecisionCommand) ([]byte, error) {
	var event map[string]any
	row := tx.QueryRow(ctx,
		`SELECT account_id, amount_cents, currency, risk_score,
			client_ip, merchant_category
		FROM payments WHERE id = $1`, cmd.PaymentId)

	var accountID, currency, clientIP, mcc string
	var amountCents int64
	var riskScore float64
	if err := row.Scan(&accountID, &amountCents, &currency, &riskScore, &clientIP, &mcc); err != nil {
		return nil, fmt.Errorf("load payment: %w", err)
	}

	event = map[string]any{
		"payment_id":        cmd.PaymentId,
		"account_id":        accountID,
		"amount_cents":      amountCents,
		"currency":          currency,
		"status":            cmd.Decision,
		"risk_score":        riskScore,
		"reasons":           []string{"human_decision:" + cmd.Operator},
		"client_ip":         clientIP,
		"merchant_category": mcc,
		"occurred_at":       cmd.DecidedAt,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return nil, fmt.Errorf("marshal event: %w", err)
	}

	return payload, nil
}