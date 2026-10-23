package adapter

import (
	"github.com/coigo/payment-processor-course/internal/plataform/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	authorizeDuration = metrics.NewHistogram(
		"ledger_authorize_duration_seconds",
		"End to end durations of one authorization",
	)
	decisions = metrics.NewCounter(
		"ledger_decisions_total",
		"Authorization outcomes by status and replay",
		"status", "replayed",
	)
	amountVolume = metrics.NewCounter(
		"ledger_amount_cents_total",
		"money volume that reached each decision, in cents",
		"status",
	)
	reviewDecisions = metrics.NewCounter(
		"ledger_review_decision_total",
		"Human review decisions and their agreement with the agent",
		"decision", "followed_recommendation",
	)
	
)

func countReviewDecision (decision string, followed bool) {
	d := "false"
	if followed {
		d = "true"
	}
	reviewDecisions.With(prometheus.Labels{"decisions": decision, "followed_recommendation": d }).Inc()
}

func countDecision (status string, replayed bool, amountCents int64) {
	r := "false"
	if replayed {
		r = "true"
	}
	decisions.With(prometheus.Labels{"status": status, "replayed": r}).Inc()
	if !replayed {
		amountVolume.With(prometheus.Labels{"status": status}).Add(float64(amountCents))
	}
}