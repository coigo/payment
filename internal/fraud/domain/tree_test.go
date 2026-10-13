package domain_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/coigo/payment-processor-course/internal/fraud/domain"
)

func TestParseTreeModelErrors(t *testing.T) {
	sentinel := domain.ErroModeInvalid

	makeErr := func(name string, data []byte) func(*testing.T) {
		return func(t *testing.T) {
			_, err := domain.ParseTreeModel(data)
			if err == nil {
				t.Fatalf("ParseTreeModel(%s) expected error, got nil", name)
			}
			if !errors.Is(err, sentinel) {
				t.Errorf("ParseTreeModel(%s) error = %v, want errors.Is(%v)", name, err, sentinel)
			}
		}
	}

	t.Run("malformed JSON", makeErr("malformed JSON", []byte(`{`)))

	t.Run("Features 0", makeErr("Features 0", mustMarshalJSON(map[string]any{
		"version":  "1",
		"features": 0,
		"baseline": 0,
		"trees":    []any{map[string]any{"nodes": []any{map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1}}}},
	})))

	t.Run("Features 9", makeErr("Features 9", mustMarshalJSON(map[string]any{
		"version":  "1",
		"features": 9,
		"baseline": 0,
		"trees":    []any{map[string]any{"nodes": []any{map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1}}}},
	})))

	t.Run("Trees empty", makeErr("Trees empty", mustMarshalJSON(map[string]any{
		"version":  "1",
		"features": 10,
		"baseline": 0,
		"trees":    []any{},
	})))

	t.Run("Node feature negative non-leaf", makeErr("Node feature negative non-leaf", mustMarshalJSON(map[string]any{
		"version":  "1",
		"features": 10,
		"baseline": 0,
		"trees": []any{map[string]any{"nodes": []any{
			map[string]any{"feature": -1, "threshold": 0.5, "left": 1, "right": 2, "value": 0.0},
			map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1},
			map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1},
		}}},
	})))

	t.Run("Node feature equal features non-leaf", makeErr("Node feature equal features non-leaf", mustMarshalJSON(map[string]any{
		"version":  "1",
		"features": 10,
		"baseline": 0,
		"trees": []any{map[string]any{"nodes": []any{
			map[string]any{"feature": 10, "threshold": 0.5, "left": 1, "right": 2, "value": 0.0},
			map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1},
			map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1},
		}}},
	})))

	t.Run("Node left out of range", makeErr("Node left out of range", mustMarshalJSON(map[string]any{
		"version":  "1",
		"features": 10,
		"baseline": 0,
		"trees": []any{map[string]any{"nodes": []any{
			map[string]any{"feature": 0, "threshold": 0.5, "left": 5, "right": 1, "value": 0.0},
			map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1},
		}}},
	})))

	t.Run("Node right out of range", makeErr("Node right out of range", mustMarshalJSON(map[string]any{
		"version":  "1",
		"features": 10,
		"baseline": 0,
		"trees": []any{map[string]any{"nodes": []any{
			map[string]any{"feature": 0, "threshold": 0.5, "left": 1, "right": 5, "value": 0.0},
			map[string]any{"feature": 0, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1},
		}}},
	})))

	t.Run("leaf node skips feature validation", func(t *testing.T) {
		_, err := domain.ParseTreeModel(mustMarshalJSON(map[string]any{
			"version":  "1",
			"features": 10,
			"baseline": 0,
			"trees": []any{map[string]any{"nodes": []any{
				map[string]any{"feature": 99, "threshold": 0.5, "left": -1, "right": -1, "value": 0.1},
			}}},
		}))
		if err != nil {
			t.Errorf("leaf node with invalid feature should not error, got: %v", err)
		}
	})

	t.Run("valid model passes", func(t *testing.T) {
		model := domain.TreeModel{
			Version:  "1",
			Features: 10,
			Baseline: 0,
			Trees: []domain.Tree{
				{
					Nodes: []domain.Node{
						{Feature: 0, ThreshHold: 100, Left: 1, Right: 2},
						{Left: -1, Value: 0.1},
						{Left: -1, Value: 0.2},
					},
				},
			},
		}
		data, _ := json.Marshal(model)
		_, err := domain.ParseTreeModel(data)
		if err != nil {
			t.Errorf("valid model should not error, got: %v", err)
		}
	})
}

func TestParseTreeModelBoundary(t *testing.T) {
	if domain.ErroModeInvalid == nil {
		t.Error("ErroModeInvalid sentinel is nil")
	}
}

func TestScoreAndEval(t *testing.T) {
	model := &domain.TreeModel{
		Version:  "1",
		Features: 10,
		Baseline: 0,
		Trees: []domain.Tree{
			{
				Nodes: []domain.Node{
					{Feature: 0, ThreshHold: 100, Left: 1, Right: 2},
					{Left: -1, Value: 0.1},
					{Left: -1, Value: 0.2},
				},
			},
			{
				Nodes: []domain.Node{
					{Feature: 1, ThreshHold: 5, Left: 3, Right: 4},
					{Left: -1, Value: 0.05},
					{Left: -1, Value: 0.15},
					{Feature: 0, ThreshHold: 200, Left: 5, Right: 6},
					{Left: -1, Value: 0.02},
					{Left: -1, Value: 0.08},
				},
			},
		},
	}

	features := make([]float64, 10)
	features[0] = 50
	features[1] = 3

	score, err := model.Score(features)
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}

	if score <= 0 || score >= 1 {
		t.Errorf("Score = %v, want between 0 and 1", score)
	}
}

func TestScoreWrongFeatureLen(t *testing.T) {
	model := &domain.TreeModel{
		Version:  "1",
		Features: 10,
		Baseline: 0,
		Trees: []domain.Tree{
			{Nodes: []domain.Node{{Left: -1, Value: 0.1}}},
		},
	}
	_, err := model.Score([]float64{1, 2, 3})
	if err == nil {
		t.Error("Score with wrong feature len expected error")
	}
	if !errors.Is(err, domain.ErroModeInvalid) {
		t.Errorf("error = %v, want errors.Is(ErroModeInvalid)", err)
	}
}

func TestSigmoidViaScore(t *testing.T) {
	smallModel := func(value float64) *domain.TreeModel {
		return &domain.TreeModel{
			Version:  "1",
			Features: 10,
			Baseline: 0,
			Trees: []domain.Tree{
				{Nodes: []domain.Node{{Left: -1, Value: value}}},
			},
		}
	}

	t.Run("score 0 sigmoid 0.5", func(t *testing.T) {
		m := smallModel(0)
		features := make([]float64, 10)
		s, err := m.Score(features)
		if err != nil {
			t.Fatalf("Score() error = %v", err)
		}
		if s != 0.5 {
			t.Errorf("Score = %v, want 0.5", s)
		}
	})

	t.Run("positive score sigmoid > 0.5", func(t *testing.T) {
		m := smallModel(0.1)
		features := make([]float64, 10)
		s, err := m.Score(features)
		if err != nil {
			t.Fatalf("Score() error = %v", err)
		}
		if s <= 0.5 {
			t.Errorf("Score = %v, want > 0.5", s)
		}
		if s >= 1 {
			t.Errorf("Score = %v, want < 1", s)
		}
	})

	t.Run("negative score sigmoid < 0.5", func(t *testing.T) {
		m := smallModel(-0.1)
		features := make([]float64, 10)
		s, err := m.Score(features)
		if err != nil {
			t.Fatalf("Score() error = %v", err)
		}
		if s >= 0.5 {
			t.Errorf("Score = %v, want < 0.5", s)
		}
	})

	t.Run("x > 30 triggers expNeg cutoff returning 1", func(t *testing.T) {
		m := smallModel(100)
		features := make([]float64, 10)
		s, err := m.Score(features)
		if err != nil {
			t.Fatalf("Score() error = %v", err)
		}
		if s != 1 {
			t.Errorf("Score = %v, want 1 (expNeg cutoff returns 0, sigmoid(0)=1)", s)
		}
	})
}

func TestEvalLeafIndexNegativeOne(t *testing.T) {
	model := &domain.TreeModel{
		Version:  "1",
		Features: 10,
		Baseline: 0,
		Trees: []domain.Tree{
			{Nodes: []domain.Node{
				{Feature: 0, ThreshHold: 50, Left: 1, Right: 2},
				{Left: -1, Value: 0.1},
				{Left: -1, Value: 0.2},
			}},
		},
	}
	features := make([]float64, 10)

	s1, _ := model.Score(features)
	features[0] = 100
	s2, _ := model.Score(features)

	if s1 == s2 {
		t.Errorf("different leaf paths should give different scores: s1=%v s2=%v", s1, s2)
	}
}

func mustMarshalJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}
