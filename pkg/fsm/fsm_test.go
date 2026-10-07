package fsm_test

import (
	"context"
	"errors"
	"testing"

	"github.com/coigo/payment-processor-course/pkg/fsm"
)

type payload struct {
	visited []string
	score   float64
}

func record(p *payload, name string) {
	p.visited = append(p.visited, name)
}

func TestRunHappyPath(t *testing.T) {
	m := fsm.New[payload]("start").
		Handle("start", func(ctx context.Context, p *payload) (fsm.State, error) {
			record(p, "start")
			return "score", nil
		}).
		Handle("score", func(ctx context.Context, p *payload) (fsm.State, error) {
			record(p, "score")
			p.score = 0.9
			return "done", nil
		}).
		Terminal("done")

	p := &payload{}
	trace, err := m.Run(context.Background(), p)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(trace) != 2 {
		t.Fatalf("expected trace len 2, got %d", len(trace))
	}
	if p.score != 0.9 {
		t.Fatalf("expected score 0.9, got %f", p.score)
	}
}

func TestFallbackKeepsPipelineAvailable(t *testing.T) {
	dependencyDown := errors.New("upstream timeout")

	m := fsm.New[payload]("start").
		Handle("start", func(ctx context.Context, p *payload) (fsm.State, error) {
			record(p, "start")
			return "ml", nil
		}).
		Handle("ml", func(ctx context.Context, p *payload) (fsm.State, error) {
			return "", dependencyDown
		}).
		Handle("rules_only", func(ctx context.Context, p *payload) (fsm.State, error) {
			record(p, "rules_only")
			return "done", nil
		}).
		FallBackTo("ml", "rules_only").
		Terminal("done")

	p := &payload{}
	_, err := m.Run(context.Background(), p)

	if err != nil {
		t.Fatalf("expected no error (fallback absorbed), got %v", err)
	}
	if len(p.visited) == 0 || p.visited[len(p.visited)-1] != "rules_only" {
		t.Fatalf("expected last visited to be rules_only, got %v", p.visited)
	}
}

func TestErrorWithoutFallbackAborts(t *testing.T) {
	boom := errors.New("boom")

	m := fsm.New[payload]("start").
		Handle("start", func(ctx context.Context, p *payload) (fsm.State, error) {
			return "", boom
		}).
		Terminal("done")

	_, err := m.Run(context.Background(), &payload{})

	if !errors.Is(err, boom) {
		t.Fatalf("expected errors.Is(err, boom) to be true, got %v", err)
	}
}
