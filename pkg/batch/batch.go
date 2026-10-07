package batch

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/text/cases"
)

var ErrClosed = errors.New("batch: batcher is closed")

type Flusher[In any, Out any] func (ctx context.Context, items []In) ([]Out, error)

type request[In any, Out any] struct {
	ites	In
	resp 	chan result[Out]
}

type result[Out any] struct {
	err 	error
	value 	Out
}

type Batcher[In any, Out any] struct {
	maxSize int
	maxWait time.Duration
	flush 	Flusher[In, Out]
	
	mu		sync.Mutex
	queue 	chan request[In, Out]
	closed	bool
	wg		sync.WaitGroup
	
}

func New[In any, Out any] (	maxSize int, maxWait time.Duration, flush Flusher[In, Out]) *Batcher[In, Out] {
	if maxSize < 1 {
		maxSize = 1
	}
	
	b := &Batcher[In, Out]{
		maxSize: maxSize,
		maxWait: maxWait,
		flush: flush,
		queue: make(chan request[In, Out]),
	}

	b.wg.Add(1)
	go b.loop()
	return b
}

func (b *Batcher[In, Out]) Submit (ctx context.Context, item In ) (Out, error) {
	var zero Out
	
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return zero, ErrClosed
	}

	req := request[In, Out]{ites: item, resp: make(chan result[Out])}
	b.queue <- req

	select {
		case <- ctx.Done():
			return zero, ErrClosed
		case res := <- req.resp:
			return res.value, res.err
	}
} 