package batch_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coigo/payment-processor-course/pkg/batch"
)

func TestSubmitAfterClose(t *testing.T) {
	echo := func(_ context.Context, items []int) ([]int, error) { return items, nil }
	b := batch.New(2, time.Millisecond, echo)
	b.Close()

	_, err := b.Submit(context.Background(), 1)
	if !errors.Is(err, batch.ErrClosed) {
		t.Fatalf("expected ErrClosed, got %v", err)
	}
}

func TestBatchBySize(t *testing.T) {
	var calls atomic.Int32
	double := func(_ context.Context, items []int) ([]int, error) {
		calls.Add(1)
		result := make([]int, len(items))
		for i, v := range items {
			result[i] = v * 2
		}
		return result, nil
	}

	b := batch.New(4, 100*time.Millisecond, double)
	defer b.Close()

	const n = 8
	results := make([]int, n)
	var wg sync.WaitGroup
	wg.Add(n)

	var ready sync.WaitGroup
	ready.Add(1)

	ctx := context.Background()
	for i := 0; i < n; i++ {
		go func(idx int) {
			defer wg.Done()
			ready.Wait()
			val, err := b.Submit(ctx, idx)
			if err != nil {
				t.Errorf("Submit(%d) failed: %v", idx, err)
				return
			}
			results[idx] = val
		}(i)
	}

	ready.Done()
	wg.Wait()

	for i := 0; i < n; i++ {
		if results[i] != i*2 {
			t.Errorf("results[%d] = %d, want %d", i, results[i], i*2)
		}
	}

	if calls.Load() > 4 {
		t.Errorf("flusher called %d times, want at most 4", calls.Load())
	}
}

func TestSubmitContextCanceledBeforeEnqueue(t *testing.T) {
	echo := func(_ context.Context, items []int) ([]int, error) { return items, nil }
	b := batch.New(2, time.Hour, echo)
	defer b.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Submit(ctx, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSubmitContextCanceledAfterEnqueue(t *testing.T) {
	echo := func(_ context.Context, items []int) ([]int, error) {
		time.Sleep(50 * time.Millisecond)
		return items, nil
	}
	b := batch.New(2, time.Hour, echo)
	defer b.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := b.Submit(ctx, 1)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestFlusherReturnsError(t *testing.T) {
	wantErr := errors.New("flusher error")
	flusher := func(_ context.Context, items []int) ([]int, error) {
		return nil, wantErr
	}
	b := batch.New(4, 10*time.Millisecond, flusher)
	defer b.Close()

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_, err := b.Submit(ctx, i)
		if !errors.Is(err, wantErr) {
			t.Errorf("Submit(%d) err = %v, want %v", i, err, wantErr)
		}
	}
}

func TestFlusherWrongResultLen(t *testing.T) {
	flusher := func(_ context.Context, items []int) ([]int, error) {
		return []int{1, 2}, nil
	}
	b := batch.New(4, 10*time.Millisecond, flusher)
	defer b.Close()

	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_, err := b.Submit(ctx, i)
		if !errors.Is(err, batch.ErrBadResultLen) {
			t.Errorf("Submit(%d) err = %v, want ErrBadResultLen", i, err)
		}
	}
}

func TestCloseTwice(t *testing.T) {
	echo := func(_ context.Context, items []int) ([]int, error) { return items, nil }
	b := batch.New(2, time.Millisecond, echo)
	b.Close()
	b.Close()
	b.Close()
}

func TestNewWithMaxSizeZero(t *testing.T) {
	var calls atomic.Int32
	echo := func(_ context.Context, items []int) ([]int, error) {
		calls.Add(1)
		return items, nil
	}
	b := batch.New(0, 10*time.Millisecond, echo)
	defer b.Close()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		b.Submit(ctx, i)
	}

	if calls.Load() != 3 {
		t.Errorf("flusher called %d times, want 3 (maxSize=0 normalized to 1, each item triggers immediate flush)", calls.Load())
	}
}

func TestLoopExitsOnQueueClose(t *testing.T) {
	var calls atomic.Int32
	echo := func(_ context.Context, items []int) ([]int, error) {
		calls.Add(1)
		return items, nil
	}
	b := batch.New(10, 10*time.Millisecond, echo)

	ctx := context.Background()
	b.Submit(ctx, 1)

	b.Close()

	if calls.Load() != 1 {
		t.Errorf("flusher called %d times after close, want 1", calls.Load())
	}
}
