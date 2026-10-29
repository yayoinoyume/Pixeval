package subscription

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// DefaultInterval is used when Start gets a non-positive interval.
const DefaultInterval = 15 * time.Minute

// Fetcher returns the newest page of works for a subscription. The caller
// implements it over pixiv.Client; this package does not know the endpoints.
type Fetcher interface {
	Fetch(ctx context.Context, s Subscription) ([]pixiv.Illustration, error)
}

// Fetched is one new work and the subscription that returned it.
type Fetched struct {
	Work         pixiv.Illustration
	Subscription Subscription
}

// Service checks subscriptions and reports works newer than the last check.
// Store holds the subscriptions and OnNew receives the new works.
type Service struct {
	// Store holds the subscriptions Check walks. It must be set before Check
	// or Start is called.
	Store *Store
	// OnNew receives each batch of new works from Start. It runs on the timer
	// goroutine, so the callback switches to the UI thread itself. Set it
	// before Start.
	OnNew func([]pixiv.Illustration)
	// OnNewFetched receives the same batches with the subscription each work
	// came from. When set, it is called instead of OnNew.
	OnNewFetched func([]Fetched)

	fetcher Fetcher
	checkMu sync.Mutex         // one check at a time, manual or timed
	seen    map[int64]struct{} // IDs already returned by any check

	mu     sync.Mutex
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// New builds a service over f.
func New(f Fetcher) *Service {
	return &Service{fetcher: f, seen: make(map[int64]struct{})}
}

// Check fetches every subscription and returns works created after its
// LastChecked that no earlier check returned. A subscription whose fetch fails
// is skipped and keeps its LastChecked, so the next check retries it. The
// error return is only for a cancelled context.
func (s *Service) Check(ctx context.Context) ([]pixiv.Illustration, error) {
	fetched, err := s.CheckDetailed(ctx)
	if len(fetched) == 0 {
		return nil, err
	}
	works := make([]pixiv.Illustration, len(fetched))
	for i, f := range fetched {
		works[i] = f.Work
	}
	return works, err
}

// CheckDetailed is Check with the subscription each work came from, so a
// caller can tell which followed target an update belongs to.
func (s *Service) CheckDetailed(ctx context.Context) ([]Fetched, error) {
	if s.Store == nil {
		return nil, errors.New("subscription: Check needs a Store")
	}
	s.checkMu.Lock()
	defer s.checkMu.Unlock()

	var fetched []Fetched
	for _, sub := range s.Store.List() {
		if err := ctx.Err(); err != nil {
			return fetched, err
		}
		items, err := s.fetcher.Fetch(ctx, sub)
		if err != nil {
			continue
		}
		var newest time.Time
		for _, w := range items {
			if w.CreateDate.After(newest) {
				newest = w.CreateDate
			}
			if !w.CreateDate.After(sub.LastChecked) {
				continue
			}
			if _, ok := s.seen[w.ID]; ok {
				continue
			}
			s.seen[w.ID] = struct{}{}
			fetched = append(fetched, Fetched{Work: w, Subscription: sub})
		}
		s.Store.setLastChecked(sub.ID, newest)
	}
	return fetched, nil
}

// Start checks immediately, then every interval, until Stop or ctx is done.
// Each batch goes to OnNewFetched, or to OnNew when that is the only callback.
// Calling Start while it already runs does nothing.
func (s *Service) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultInterval
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.wg.Add(1)
	s.mu.Unlock()
	go s.loop(ctx, interval)
}

// Stop ends a started loop and waits for it, so no OnNew runs after it
// returns. It is safe to call when Start was never called.
func (s *Service) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.cancel = nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	s.wg.Wait()
}

func (s *Service) loop(ctx context.Context, interval time.Duration) {
	defer s.wg.Done()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if fetched, err := s.CheckDetailed(ctx); err == nil && len(fetched) > 0 {
			s.report(fetched)
		}
		select {
		case <-ctx.Done():
			s.mu.Lock()
			s.cancel = nil
			s.mu.Unlock()
			return
		case <-ticker.C:
		}
	}
}

// report hands a batch to the detailed callback, or to OnNew when only the
// works-only callback is set.
func (s *Service) report(fetched []Fetched) {
	if s.OnNewFetched != nil {
		s.OnNewFetched(fetched)
		return
	}
	if s.OnNew == nil {
		return
	}
	works := make([]pixiv.Illustration, len(fetched))
	for i, f := range fetched {
		works[i] = f.Work
	}
	s.OnNew(works)
}
