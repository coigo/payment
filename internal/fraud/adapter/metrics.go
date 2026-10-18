package adapter

import (
	"strings"

	"github.com/coigo/payment-processor-course/internal/plataform/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)


var scoreDuration = metrics.NewHistogram(
	"fraud_score_duration_seconds", 
	"Total duration of one scoring call.",
)

var stageDuration = metrics.NewHistogram(
	"fraud_stage_duration_seconds", 
	"Duration of each FSM stage, from the line trace. .",
	"stage",
)

var verdicts = metrics.NewCounter(
	"fraud_verdicts_total", 
	"Scoring verdicts by outcode and degrarion.",
	"verdict", "degraded",
)

var reasons = metrics.NewCounter(
	"fraud_reasons_total", 
	"Decision reasonsemitted by the pipeline stages.",
	"reason",
)

var riskScore = promauto.NewHistogram(prometheus.HistogramOpts{
	Name: "fraud_risk_score",
	Help: "Distribution of model rist scores for scored attempts.",
	Buckets: prometheus.LinearBuckets(0.05, 0.05, 19),
	
})

func observeStage (stage string, secconts float64) {
	stageDuration.WithLabelValues(stage).Observe(secconts)
}

func countVerdict (verdict string, degraded bool) {
	d := "false"
	if degraded {
		d = "true"
	}
	verdicts.With(prometheus.Labels{"verdict": verdict, "degraded": d})
}

func countReasons (list []string) {

	for _, reason := range list {
		if idx := strings.IndexByte(reason, ':'); idx > 0 {
			reason = reason[:idx]
		}
		reasons.With(prometheus.Labels{"reason": reason}).Inc()
	}

}

func observeRiskScore (score float64) {
	if score == 0 {
		riskScore.Observe(score)
	}
}
