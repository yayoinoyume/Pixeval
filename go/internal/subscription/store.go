// Package subscription stores update subscriptions and checks them for new
// works. It does not import the UI or the download queue.
package subscription

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// maxSubscriptions is the most entries the store keeps, so a bad file or a
// runaway loop cannot make every check slower and slower.
const maxSubscriptions = 500

// Kind is what a subscription follows.
type Kind string

const (
	KindUser   Kind = "user"   // TargetID is a user ID
	KindTag    Kind = "tag"    // TargetID is a bookmark tag or 0 for all
	KindSeries Kind = "series" // TargetID is an illustration series ID
)

// Subscription is one followed target. LastChecked is the CreateDate of the
// newest work a check has seen; works at or before it are not new.
type Subscription struct {
	ID          string    `json:"id"`
	Kind        Kind      `json:"kind"`
	TargetID    int64     `json:"target_id"`
	TargetName  string    `json:"target_name"`
	LastChecked time.Time `json:"last_checked"`
}

// Store is the subscription list. Subscriptions is exported for loading and
// saving; read and change it through the methods, which hold mu.
type Store struct {
	mu            sync.Mutex
	Subscriptions []Subscription
}

// subscriptionFile is the on-disk shape, so the format can grow later.
type subscriptionFile struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

// Load replaces the list with the one at path. It returns the error and keeps
// the store unchanged when the file is missing or is not valid JSON, so the
// caller can start empty and report the problem.
func (st *Store) Load(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file subscriptionFile
	if err := json.Unmarshal(data, &file); err != nil {
		return fmt.Errorf("subscription: decode %s: %w", path, err)
	}
	subs := file.Subscriptions
	if len(subs) > maxSubscriptions {
		subs = subs[:maxSubscriptions]
	}
	st.mu.Lock()
	st.Subscriptions = subs
	st.mu.Unlock()
	return nil
}

// Save writes the list to path as JSON only the user can read, replacing the
// file in one step so a crash cannot leave a half written one.
func (st *Store) Save(path string) error {
	st.mu.Lock()
	subs := append([]Subscription(nil), st.Subscriptions...)
	st.mu.Unlock()
	data, err := json.MarshalIndent(subscriptionFile{Subscriptions: subs}, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".subscriptions-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Add appends sub and reports whether it was kept. A duplicate ID or a full
// store is refused.
func (st *Store) Add(sub Subscription) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for _, existing := range st.Subscriptions {
		if existing.ID == sub.ID {
			return false
		}
	}
	if len(st.Subscriptions) >= maxSubscriptions {
		return false
	}
	st.Subscriptions = append(st.Subscriptions, sub)
	return true
}

// Remove drops the subscription with id and reports whether it was present.
func (st *Store) Remove(id string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i, sub := range st.Subscriptions {
		if sub.ID == id {
			st.Subscriptions = append(st.Subscriptions[:i], st.Subscriptions[i+1:]...)
			return true
		}
	}
	return false
}

// List returns a copy of the subscriptions, safe to keep after the call.
func (st *Store) List() []Subscription {
	st.mu.Lock()
	defer st.mu.Unlock()
	return append([]Subscription(nil), st.Subscriptions...)
}

// setLastChecked advances one subscription, never rewinding it.
func (st *Store) setLastChecked(id string, t time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	for i := range st.Subscriptions {
		if st.Subscriptions[i].ID == id && t.After(st.Subscriptions[i].LastChecked) {
			st.Subscriptions[i].LastChecked = t
		}
	}
}
