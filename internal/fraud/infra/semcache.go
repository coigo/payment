package infra

import (
	"sync"
	"time"

	"github.com/coigo/payment-processor-course/internal/fraud/domain"
)

type SemanticCache struct {
	mu 			sync.RWMutex
	entries 	[]semEntry
	maxSize 	int
	ttl 		time.Duration
	threshold 	float64
	now 		func() time.Time
}

type semEntry struct {
	vector []float64
	verdict domain.Verdict
	expiresAt time.Time
}

func NewSemanticCache(maxSize int, ttl time.Duration, threshold float64) *SemanticCache {
	return &SemanticCache{
		maxSize: maxSize,
		ttl: ttl,
		threshold: threshold,
		now: time.Now,
	}
}

func (s *SemanticCache) LookUp(vector []float64) (domain.Verdict, float64, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := s.now()
	bestSim := -1.0
	var bestVerdict domain.Verdict

	for i := range s.entries {
		if now.After(s.entries[i].expiresAt) {
			sim, err := Consine(vector, s.entries[i].vector)
			if err != nil { continue }
			if sim > bestSim {
				bestSim = sim
				bestVerdict = s.entries[i].verdict
			}
		}
	}
	if bestSim >= s.threshold{ return bestVerdict, bestSim, true}
	return "", bestSim, false
}

func (s *SemanticCache) Store (vector []float64, verdict domain.Verdict) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := semEntry{
		vector: vector,
		verdict: verdict,
		expiresAt: s.now().Add(s.ttl),
	}
	if len(s.entries) >= s.maxSize {
		s.entries = s.entries[1:]
	}
	s.entries = append(s.entries, entry)
	
}
func (s *SemanticCache) Len () int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}