package batch

import (
	"context"
	"errors"
	"sync"
	"time"

)

var ErrClosed = errors.New("batch: batcher is closed")

type Flusher[In any, Out any] func (ctx context.Context, items []In) ([]Out, error)

type request[In any, Out any] struct {
	items	In
	resp 	chan result[Out]
}

type result[Out any] struct {
	err 	error
	value 	Out
}

type Batcher[In any, Out any] struct {
	maxSize int
	maxWait time.Duration
	flush   Flusher[In, Out]

	mu     sync.RWMutex
	queue  chan request[In, Out]
	closed bool
	wg     sync.WaitGroup
}

func New[In any, Out any] (    maxSize int, maxWait time.Duration, flush Flusher[In, Out]) *Batcher[In, Out] {
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

func (b *Batcher[In, Out]) Submit(ctx context.Context, item In) (Out, error) {
	var zero Out
	req := request[In, Out]{
		items: item,
		resp:  make(chan result[Out], 1), // buffer: dispatch nunca bloqueia
	}

	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return zero, ErrClosed
	}
	select {
	case b.queue <- req:
		b.mu.RUnlock()
	case <-ctx.Done():
		b.mu.RUnlock()
		return zero, ctx.Err()
	}

	select {
	case <-ctx.Done():
		return zero, ctx.Err()
	case res := <-req.resp:
		return res.value, res.err
	}
}

func (b *Batcher[In, Out]) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	close(b.queue)
	b.mu.Unlock()

	b.wg.Wait()
}

var ErrBadResultLen = errors.New("batch: flusher returned wrong number of results")

func (b *Batcher[In, Out]) dispatch(batch []request[In, Out]) {
	items := make([]In, len(batch))
	for i, req := range batch {
		items[i] = req.items
	}

	outs, err := b.flush(context.Background(), items)
	if err == nil && len(outs) != len(items) {
		err = ErrBadResultLen
	}

	for i, req := range batch {
		if err != nil {
			req.resp <- result[Out]{err: err}
			continue
		}
		req.resp <- result[Out]{value: outs[i]}
	}
}

func (b *Batcher[In, Out]) loop() {
	defer b.wg.Done()

	pending := make([]request[In, Out], 0, b.maxSize)

	timer := time.NewTimer(b.maxWait)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	flushPending := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		if len(pending) == 0 {
			return
		}
		batch := pending
		pending = make([]request[In, Out], 0, b.maxSize)
		b.dispatch(batch)
	}

	for {
		select {
		case req, ok := <-b.queue:
			if !ok {
				flushPending()
				return
			}
			pending = append(pending, req)
			if len(pending) == 1 {
				timer.Reset(b.maxWait)
			}
			if len(pending) >= b.maxSize {
				flushPending()
			}
		case <-timer.C:
			flushPending()
		}
	}
}