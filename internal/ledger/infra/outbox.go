package infra

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/coigo/payment-processor-course/internal/plataform/metrics"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
)

var outboxUnpublished = metrics.NewGauge (
	"ledger_outbox_unpublished",
	"Outbox rows waiting for publication")

var reviewQueue = metrics.NewGaugeVec(
	"ledger_reviews",
	"Review cases by current status",
	"status")

type OutboxRelay struct {
	pool 		*pgxpool.Pool
	writers 	map[string]*kafka.Writer
	logger 		*slog.Logger
	every 		time.Duration
	batch 		int
}

func NewOutboxRelay (pool *pgxpool.Pool, writers map[string]*kafka.Writer, logger *slog.Logger, every time.Duration, batch int) *OutboxRelay{
	return &OutboxRelay{
		pool: pool,
		writers: writers,
		logger: logger,
		every: every,
		batch: batch,
	}
}

func (r *OutboxRelay) Run (ctx context.Context) {
	ticker := time.NewTicker(r.every)
	defer ticker.Stop()

	gaugeTicker := time.NewTicker(5 * time.Second)
	defer gaugeTicker.Stop()
	
	for {
		select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := r.drain(ctx); err != nil && ctx.Err() == nil{
					r.logger.Error( "failed to drain outbox", "err", err)
				}
			case <- gaugeTicker.C:
			var pending int64
			if err := r.pool.QueryRow(ctx, 					
				`SELECT count(*) FROM outbox WHERE published_at IS NULL`).Scan(&pending); err == nil {
				outboxUnpublished.Set(float64(pending))
			}
			r.observeReviewQueue(ctx)
		}
	}
}

func (r *OutboxRelay) observeReviewQueue (ctx context.Context) {
	for _, status := range []string{"pending", "approved", "rejected"} {
		reviewQueue.WithLabelValues(status).Set(0)
	}

	rows, err := r.pool.Query(ctx, "SELECT status, COUNT(*) from reviews GROUP BY status")
	if err != nil {
		return 
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil { return }
		reviewQueue.WithLabelValues(status).Set(float64(count))
	}
}

type outboxRow struct {
	id int64
	topic string
	key string
	payload []byte
}

func (r *OutboxRelay) drain(ctx context.Context) error {
	rows, err := r.pool.Query(ctx,
		`SELECT id, topic, key, payload FROM outbox
		WHERE published_at IS NULL ORDER BY id LIMIT $1`, r.batch)
	if err != nil {
		return err
	}
	defer rows.Close()

	var pending []outboxRow
	for rows.Next() {
		var row outboxRow
		if err := rows.Scan(&row.id, &row.topic, &row.key, &row.payload); err != nil {
			return err
		}

		pending = append(pending, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil { return err }

	for _, row := range pending {
		writer, ok := r.writers[row.topic]
		if !ok {
			r.logger.Error("no writer for topic", "topic", row.topic)
			continue
		}

		if err := writer.WriteMessages(ctx, kafka.Message{
			Key:   []byte(row.key),
			Value: row.payload,
		}); err != nil {
			r.logger.Warn("failed to publish message", "topic", row.topic, "error", err)
		}
		
		msg := kafka.Message{
			Key: []byte(row.key),
			Value: row.payload,
		}
		if err := writer.WriteMessages(ctx, msg); err != nil {
			return fmt.Errorf("failed to publish messages: %w", err)
		}
		if _, err := r.pool.Exec(ctx, "UPDATE outbox SET published_at = NOW() WHERE id = $1", row.id); err != nil {
					return fmt.Errorf("failed to update outbox row: %w", err)
		}
	}
	return nil
}