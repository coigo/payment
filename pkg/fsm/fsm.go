package fsm

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type State string

var ErrNoTransition = errors.New("no transition registered for current state")

type Handler[T any] func(ctx context.Context, data *T) (State, error)

type Transition struct {
	From     State
	To       State
	Duration time.Duration
	Err      error
}	

type Machine[T any] struct {
	initial 	State
	terminals 	map[State]bool
	handlers 	map[State]Handler[T]
	fallBacks	map[State]State
	maxHops		int
}

type Option[T any] func(*Machine[T])

func WithMaxHops[T any](n int) Option[T] {
	return func(m *Machine[T]) {
		m.maxHops = n
	}
}

func New[T any](initial State, opts ...Option[T]) *Machine[T] {
	m := &Machine[T] {
		initial: initial,
		terminals: make(map[State]bool),
		handlers: map[State]Handler[T]{},
		fallBacks: map[State]State{},
		maxHops: 32,
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

func (m *Machine[T]) Handle(state State, h Handler[T]) *Machine[T] {
	m.handlers[state] = h
	return m 
}

func (m *Machine[T]) Terminal(states ...State) *Machine[T] {
	for _, s := range states {
		m.terminals[s] = true
	}
	return m
}

func (m *Machine[T]) FallBackTo(state State, target State) *Machine[T] {
	m.fallBacks[state] = target
	return m
}

func (m *Machine[T]) Run (ctx context.Context , data *T) ([]Transition, error) {
	trace := []Transition{}
	currState := m.initial
	for i := 0; i < m.maxHops; i++  {
		if ctx.Err() != nil {
			return trace, fmt.Errorf("fsm: context canceled at %q: %w", currState, ctx.Err()) 
		}

		handler, ok := m.handlers[currState]
		if !ok {
			return trace, fmt.Errorf("%w: %q", ErrNoTransition, currState) 
		}

		start := time.Now()
		next, err := handler(ctx, data)
		dutration := time.Since(start)

		hop := Transition{
			From: currState,
			To: next,
			Duration: dutration,
			Err: err,
		}
		trace = append(trace, hop)

		if err != nil {
			fallBack, hasFallBack := m.fallBacks[currState]
			if !hasFallBack {
				return trace, fmt.Errorf("%w: %q: %w", ErrNoTransition, currState, err)
			}

			hop.To = fallBack
			next = fallBack
		}
	
		if m.terminals[next] {
			return trace, nil
		}
		currState = next
	}
	return trace, fmt.Errorf("fsm: exeeded %d hops starting at %q", m.maxHops, m.initial)
}