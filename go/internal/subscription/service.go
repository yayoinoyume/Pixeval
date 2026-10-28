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
	if s.Store == nil {
		return nil, errors.New("subscription: Check needs a Store")
	}
	s.checkMu.Lock()
	defer s.checkMu.Unlock()

	var works []pixiv.Illustration
	for _, sub := range s.Store.List() {
		if err := ctx.Err(); err != nil {
			return works, err
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
			works = append(works, w)
		}
		s.Store.setLastChecked(sub.ID, newest)
	}
	return works, nil
}

// Start checks immediately, then every interval, until Stop or ctx is done.
// Each batch goes to OnNew. Calling Start while it already runs does nothing.
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
		if works, err := s.Check(ctx); err == nil && len(works) > 0 && s.OnNew != nil {
			s.OnNew(works)
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
