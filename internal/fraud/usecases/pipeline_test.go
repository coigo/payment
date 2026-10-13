package usecases_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/coigo/payment-processor-course/internal/fraud/domain"
	"github.com/coigo/payment-processor-course/internal/fraud/usecases"
	"github.com/coigo/payment-processor-course/pkg/fsm"
)

type fakeGeo struct {
	country string
}

func (f fakeGeo) CountryCode(ctx context.Context, ip string) (string, error) {
	return f.country, nil
}

type fakeEmbedder struct {
	vector []float64
}

func (f fakeEmbedder) Embed(text string) []float64 {
	return f.vector
}

type fakeSemCache struct {
	store map[string]struct {
		verdict    domain.Verdict
		similarity float64
	}
}

func (f *fakeSemCache) LookUp(vector []float64) (domain.Verdict, float64, bool) {
	key := vectorKey(vector)
	entry, ok := f.store[key]
	if !ok {
		return "", 0, false
	}
	return entry.verdict, entry.similarity, true
}

func (f *fakeSemCache) Store(vector []float64, verdict domain.Verdict) {
	key := vectorKey(vector)
	f.store[key] = struct {
		verdict    domain.Verdict
		similarity float64
	}{verdict: verdict, similarity: 0.95}
}

func vectorKey(v []float64) string {
	key := ""
	for _, f := range v {
		key += string(rune(int(f * 100)))
	}
	return key
}

func baseAttempt() domain.Attempt {
	return domain.Attempt{
		PaymentId:       "pay_test_123",
		AccountId:       "acc_test_456",
		CardFingerPrint: "fp_legit_card",
		AmountCents:     10000,
		Currency:        "BRL",
		ClientIP:        "192.168.1.1",
		MerchantCategory: "retail",
		OutcurredAt:     time.Now(),
	}
}

type expectedDecision struct {
	verdict   domain.Verdict
	riskScore float64
	reasons   []string
	degraded  bool
}

type testCase struct {
	name          string
	attempt       domain.Attempt
	expected      expectedDecision
	modelScore    float64
	cacheHit      bool
	cacheVerdict  domain.Verdict
	transitions   []fsm.State
	warmupCalls   int
}

func fixedScoreModel(score float64) *domain.TreeModel {
	return &domain.TreeModel{
		Version:  "test",
		Features: 5,
		Baseline: 0,
		Trees: []domain.Tree{
			{
				Nodes: []domain.Node{
					{Feature: -1, Left: -1, Value: score},
				},
			},
		},
	}
}

func (tc testCase) run(t *testing.T) {
	embedderVector := []float64{0.1, 0.2, 0.3, 0.4, 0.5}
	cache := &fakeSemCache{
		store: make(map[string]struct {
			verdict    domain.Verdict
			similarity float64
		}),
	}

	if tc.cacheHit {
		cache.store[vectorKey(embedderVector)] = struct {
			verdict    domain.Verdict
			similarity float64
		}{verdict: tc.cacheVerdict, similarity: 0.95}
	}

	geo := fakeGeo{country: "BR"}
	embedder := fakeEmbedder{vector: embedderVector}

	model := fixedScoreModel(tc.modelScore)

	pipeline := usecases.NewPipeline(
		model,
		geo,
		embedder,
		cache,
		slog.Default(),
		1,
		10*time.Millisecond,
	)
	defer pipeline.Close()

	if tc.warmupCalls > 0 {
		ctx := context.Background()
		warmupAttempt := tc.attempt
		warmupAttempt.AmountCents = 100
		for i := 0; i < tc.warmupCalls; i++ {
			_, _, err := pipeline.Score(ctx, warmupAttempt)
			if err != nil {
				t.Fatalf("warmup call %d failed: %v", i, err)
			}
		}
	}

	ctx := context.Background()
	decision, trace, err := pipeline.Score(ctx, tc.attempt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if decision.Verdict != tc.expected.verdict {
		t.Errorf("verdict: got %s, want %s", decision.Verdict, tc.expected.verdict)
	}

	if decision.Riskscore != tc.expected.riskScore {
		t.Errorf("riskScore: got %f, want %f", decision.Riskscore, tc.expected.riskScore)
	}

	for _, reason := range tc.expected.reasons {
		found := false
		for _, r := range decision.Reasons {
			if r == reason {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing reason %q, got reasons: %v", reason, decision.Reasons)
		}
	}

	if decision.Degraded != tc.expected.degraded {
		t.Errorf("degraded: got %v, want %v", decision.Degraded, tc.expected.degraded)
	}

	if len(trace) != len(tc.transitions) {
		t.Errorf("transition count: got %d, want %d", len(trace), len(tc.transitions))
		t.Errorf("trace: %v", trace)
		return
	}

	for i, expectedState := range tc.transitions {
		if trace[i].To != expectedState {
			t.Errorf("transition[%d]: got %s, want %s", i, trace[i].To, expectedState)
		}
	}
}

var pipelineTests = []testCase{
	{
		name:    "fast_rules blocklist final",
		attempt: withCardFingerprint(baseAttempt(), "fp_stolen_card_001"),
		expected: expectedDecision{
			verdict:   domain.VerdictDeny,
			riskScore: 0,
			reasons:   []string{"card_blocklisted"},
			degraded:  false,
		},
		modelScore: 0.80,
		cacheHit:   false,
		transitions: []fsm.State{
			usecases.StateFastRules,
			usecases.StateDone,
		},
	},
	{
		name:    "fast_rules pass ml confident approve",
		attempt: baseAttempt(),
		expected: expectedDecision{
			verdict:   domain.VerdictApprove,
			riskScore: 0.2689414213699952,
			reasons:   []string{"fast_rules_passed", "ml_score_confident"},
			degraded:  false,
		},
		modelScore: -1,
		cacheHit:   false,
		transitions: []fsm.State{
			usecases.StateFastRules,
			usecases.StateMLScore,
			usecases.StateFinalize,
			usecases.StateDone,
		},
	},
	{
		name:    "fast_rules pass ml confident deny",
		attempt: baseAttempt(),
		expected: expectedDecision{
			verdict:   domain.VerdictDeny,
			riskScore: 0.8807970779778823,
			reasons:   []string{"fast_rules_passed", "ml_score_confident"},
			degraded:  false,
		},
		modelScore: 2,
		cacheHit:   false,
		transitions: []fsm.State{
			usecases.StateFastRules,
			usecases.StateMLScore,
			usecases.StateFinalize,
			usecases.StateDone,
		},
	},
	{
		name:    "ml ambiguous tiebreak cache hit approve",
		attempt: baseAttempt(),
		expected: expectedDecision{
			verdict:   domain.VerdictApprove,
			riskScore: 0.5,
			reasons:   []string{"fast_rules_passed", "semantic_cache_hit: 0.95"},
			degraded:  false,
		},
		modelScore:   0,
		cacheHit:     true,
		cacheVerdict: domain.VerdictApprove,
		transitions: []fsm.State{
			usecases.StateFastRules,
			usecases.StateMLScore,
			usecases.StateTiebreak,
			usecases.StateFinalize,
			usecases.StateDone,
		},
	},
	{
		name:    "ml ambiguous tiebreak cache hit deny",
		attempt: baseAttempt(),
		expected: expectedDecision{
			verdict:   domain.VerdictDeny,
			riskScore: 0.5,
			reasons:   []string{"fast_rules_passed", "semantic_cache_hit: 0.95"},
			degraded:  false,
		},
		modelScore:   0,
		cacheHit:     true,
		cacheVerdict: domain.VerdictDeny,
		transitions: []fsm.State{
			usecases.StateFastRules,
			usecases.StateMLScore,
			usecases.StateTiebreak,
			usecases.StateFinalize,
			usecases.StateDone,
		},
	},
	{
		name:    "ml ambiguous tiebreak cache miss",
		attempt: baseAttempt(),
		expected: expectedDecision{
			verdict:   domain.VerdictManualReview,
			riskScore: 0.5,
			reasons:   []string{"fast_rules_passed", "ambiguous_score_manual_review"},
			degraded:  false,
		},
		modelScore: 0,
		cacheHit:   false,
		transitions: []fsm.State{
			usecases.StateFastRules,
			usecases.StateMLScore,
			usecases.StateTiebreak,
			usecases.StateFinalize,
			usecases.StateDone,
		},
	},
	{
		name:    "fast_rules velocity final",
		attempt: baseAttempt(),
		expected: expectedDecision{
			verdict:   domain.VerdictDeny,
			riskScore: 0,
			reasons:   []string{"velocity_limit_exceeded"},
			degraded:  false,
		},
		modelScore:   0.5,
		cacheHit:     false,
		warmupCalls:  11,
		transitions: []fsm.State{
			usecases.StateFastRules,
			usecases.StateDone,
		},
	},
}

func TestPipeline(t *testing.T) {
	for _, tc := range pipelineTests {
		t.Run(tc.name, func(t *testing.T) {
			tc.run(t)
		})
	}
}

func withCardFingerprint(a domain.Attempt, fp string) domain.Attempt {
	a.CardFingerPrint = fp
	return a
}
