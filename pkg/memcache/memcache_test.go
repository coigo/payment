package memcache_test

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/coigo/payment-processor-course/pkg/memcache"
)

func TestSetGet(t *testing.T) {
	c := memcache.New[string](time.Minute, time.Minute)
	defer c.Close()

	c.Set("k", "v")
	v, ok := c.Get("k")
	if !ok {
		t.Fatal("expected ok=true, got ok=false")
	}
	if v != "v" {
		t.Fatalf("expected v=%q, got v=%q", "v", v)
	}
}

func TestExpiration(t *testing.T) {
	c := memcache.New[int](10*time.Millisecond, time.Hour)
	defer c.Close()

	c.Set("k", 42)
	time.Sleep(30 * time.Millisecond)

	_, ok := c.Get("k")
	if ok {
		t.Fatal("expected ok=false (entry expired), got ok=true")
	}
}

func TestIncrementIsAtomic(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	defer c.Close()

	add := func(current int, exists bool) int {
		if !exists {
			return 1
		}
		return current + 1
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.Increment("card:123", add)
		}()
	}
	wg.Wait()

	v, ok := c.Get("card:123")
	if !ok {
		t.Fatal("expected ok=true, got ok=false")
	}
	if v != 100 {
		t.Fatalf("expected v=100, got v=%d", v)
	}
}

func TestConcurrentAccessAcrossShards(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	defer c.Close()

	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i%50)
			c.Set(key, i)
			c.Get(key)
		}(i)
	}
	wg.Wait()
}

func TestGetNonExistentKey(t *testing.T) {
	c := memcache.New[string](time.Minute, time.Minute)
	defer c.Close()

	_, ok := c.Get("nonexistent")
	if ok {
		t.Fatal("expected ok=false for nonexistent key, got ok=true")
	}
}

func TestOverwriteKey(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	defer c.Close()

	c.Set("k", 10)
	c.Set("k", 20)

	v, ok := c.Get("k")
	if !ok {
		t.Fatal("expected ok=true, got ok=false")
	}
	if v != 20 {
		t.Fatalf("expected v=20 after overwrite, got v=%d", v)
	}
}

func TestIncrementNonExistentKey(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	defer c.Close()

	add := func(current int, exists bool) int {
		if !exists {
			return 1
		}
		return current + 1
	}

	c.Increment("new-key", add)
	v, ok := c.Get("new-key")
	if !ok {
		t.Fatal("expected ok=true, got ok=false")
	}
	if v != 1 {
		t.Fatalf("expected v=1 for increment on nonexistent key, got v=%d", v)
	}
}

func TestZeroValue(t *testing.T) {
	c := memcache.New[int](time.Minute, time.Minute)
	defer c.Close()

	c.Set("zero", 0)
	v, ok := c.Get("zero")
	if !ok {
		t.Fatal("expected ok=true for zero value, got ok=false")
	}
	if v != 0 {
		t.Fatalf("expected v=0, got v=%d", v)
	}
}

func TestJanitorEvictsExpired(t *testing.T) {
	c := memcache.New[int](time.Millisecond, time.Millisecond)
	defer c.Close()

	for i := 0; i < 10000; i++ {
		c.Set(fmt.Sprintf("key-%d", i), i)
	}

	for c.Len() > 0 {
		runtime.Gosched()
	}

	if n := c.Len(); n != 0 {
		t.Fatalf("expected 0 entries after janitor sweep, got %d", n)
	}
}
