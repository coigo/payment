package usecases

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/coigo/payment-processor-course/internal/fraud/domain"
	"github.com/coigo/payment-processor-course/pkg/batch"
	"github.com/coigo/payment-processor-course/pkg/fsm"
	"github.com/coigo/payment-processor-course/pkg/memcache"
)

const (
	StateEnrich    = "enrich"
	StateFastRules = "fast_rules"
	StateMLScore   = "ml_score"
	StateTiebreak  = "tiebreak"
	StateFinalize  = "finalize"
	StateDone      = "done"
)

type GeoResolver interface {
	CountryCode(ctx context.Context, ip string) (string, error)
}

type Embedder interface {
	Embed(text string) []float64
}

type SemanticCache interface {
	LookUp(vector []float64) (domain.Verdict, float64, bool)
	Store(vector []float64, verdict domain.Verdict)
}

type Decision struct {
	Verdict   domain.Verdict
	Riskscore float64
	Reasons   []string
	Degraded  bool
}

type scoringData struct {
	attempt  domain.Attempt
	verdict  domain.Verdict
	vector   []float64
	cacheHit bool
}

type Pipeline struct {
	machine  *fsm.Machine[scoringData]
	model    *domain.TreeModel
	geo      GeoResolver
	embedder Embedder
	semCache SemanticCache
	velocity *memcache.Cache[int]
	batcher  *batch.Batcher[[]float64, float64]
	logger   *slog.Logger
}

func NewPipeline(
	model *domain.TreeModel,
	geo GeoResolver,
	embedder Embedder,
	semCache SemanticCache,
	logger *slog.Logger,
	batchSize int,
	batchWait time.Duration,
) *Pipeline {
	p := &Pipeline{
		model:    model,
		geo:      geo,
		embedder: embedder,
		semCache: semCache,
		velocity: memcache.New[int](time.Minute, 30*time.Second),
		logger:   logger,
	}

	p.batcher = batch.New(batchSize, batchWait, func(_ context.Context, vectors [][]float64) ([]float64, error) {
		scores := make([]float64, len(vectors))
		for i, v := range vectors {
			s, err := p.model.Score(p.padFeatures(v))
			if err != nil {
				p.logger.Warn("model scoring failed", "error", err)
				return nil, err
			}
			scores[i] = s
		}
		return scores, nil
	})

	p.machine = fsm.New[scoringData](StateEnrich).
		Handle(StateEnrich, p.enrich).
		Handle(StateFastRules, p.fastRules).
		Handle(StateMLScore, p.mlScore).
		FallBackTo(StateMLScore, StateFinalize).
		Handle(StateTiebreak, p.tiebreak).
		Handle(StateFinalize, p.finalize).
		Terminal(StateDone)

	return p
}

func (p *Pipeline) Close() {
	p.batcher.Close()
	p.velocity.Close()
}

func (p *Pipeline) padFeatures(v []float64) []float64 {
	if len(v) >= p.model.Features {
		return v
	}
	out := make([]float64, p.model.Features)
	copy(out, v)
	return out
}

func (p *Pipeline) Score(ctx context.Context, attempt domain.Attempt) (Decision, []fsm.Transition, error) {
	data := scoringData{attempt: attempt}
	trace, err := p.machine.Run(ctx, &data)
	if err != nil {
		return Decision{}, nil, fmt.Errorf("fraud pipeline: %w", err)
	}

	return Decision{
		Verdict:   data.verdict,
		Riskscore: data.attempt.RistScore, // nome definido em domain.Attempt; renomeie lá para RiskScore
		Reasons:   data.attempt.Reasons,
		Degraded:  data.attempt.Degraded,
	}, trace, nil
}

func (p *Pipeline) enrich(ctx context.Context, d *scoringData) (fsm.State, error) {
	// Increment retorna apenas o valor (não há erro a tratar).
	// Se na sua implementação ele retornar (int, error), capture os dois.
	d.attempt.VelocityLastMin = p.velocity.Increment(
		"card:"+d.attempt.CardFingerPrint,
		func(current int, exists bool) int {
			if !exists {
				return 1
			}
			return current + 1
		})

	code, err := p.geo.CountryCode(ctx, d.attempt.ClientIP)
	if err != nil {
		p.logger.Warn("geo lookup failed",
			slog.String("payment_id", d.attempt.PaymentId),
			slog.String("error", err.Error()))
		code = ""
	}
	d.attempt.CountryCode = code

	return StateFastRules, nil
}

func (p *Pipeline) fastRules(_ context.Context, d *scoringData) (fsm.State, error) {
	result := domain.ApplyFastRules(d.attempt)
	if result.Reason != "" {
		d.attempt.Reasons = append(d.attempt.Reasons, result.Reason)
	}

	if result.Final {
		d.verdict = result.Verdict
		return StateDone, nil
	}
	return StateMLScore, nil
}

func (p *Pipeline) mlScore(ctx context.Context, d *scoringData) (fsm.State, error) {
	features := domain.FeatureVector(d.attempt)

	score, err := p.batcher.Submit(ctx, features)
	if err != nil {
		// Estado degradado preparado; o FallBackTo(StateMLScore, StateFinalize)
		// do FSM deve interceptar este erro e seguir para finalize.
		d.attempt.Degraded = true
		d.attempt.Reasons = append(d.attempt.Reasons, "ml_stage_skipped")
		d.verdict = domain.VerdictManualReview
		return "", fmt.Errorf("ml score: %w", err)
	}

	d.attempt.RistScore = score // idem: renomear no domain
	d.verdict = domain.VeredictFromScore(score) // idem: renomear para VerdictFromScore no domain

	if d.verdict == domain.VerdictManualReview {
		return StateTiebreak, nil
	}

	d.attempt.Reasons = append(d.attempt.Reasons, "ml_score_confident")
	return StateFinalize, nil
}

func (p *Pipeline) tiebreak(_ context.Context, d *scoringData) (fsm.State, error) {
	// Sem CardFingerPrint: é um identificador único (ruído para similaridade)
	// e dado sensível que não deve ir para o embedder.
	summary := fmt.Sprintf("%s bucket: %d country: %s",
		d.attempt.MerchantCategory,
		d.attempt.AmountCents/50000,
		d.attempt.CountryCode,
	)

	d.vector = p.embedder.Embed(summary)
	verdict, similarity, hit := p.semCache.LookUp(d.vector)
	if hit {
		d.verdict = verdict
		d.cacheHit = true
		d.attempt.Reasons = append(d.attempt.Reasons, fmt.Sprintf("semantic_cache_hit: %.2f", similarity))
		return StateFinalize, nil
	}

	d.attempt.Reasons = append(d.attempt.Reasons, "ambiguous_score_manual_review")
	return StateFinalize, nil
}

func (p *Pipeline) finalize(_ context.Context, d *scoringData) (fsm.State, error) {
	if d.verdict == "" {
		d.verdict = domain.VerdictManualReview
	}

	// Só armazena vereditos definitivos e que não vieram do próprio cache.
	// Guardar ManualReview "envenenaria" o cache com o próprio limbo.
	if d.vector != nil && !d.cacheHit && d.verdict != domain.VerdictManualReview {
		p.semCache.Store(d.vector, d.verdict)
	}

	return StateDone, nil
}