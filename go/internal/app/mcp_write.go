package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/subscription"
)

// SetWatchLater adds or removes a work from the local watch-later list; the
// MCP watch later write tool. Adding reads the work so the entry carries its
// title, artist and cover.
func (a *App) SetWatchLater(ctx context.Context, id int64, on bool) error {
	if a.watchLater == nil {
		return errors.New("watch later is unavailable")
	}
	if !on {
		a.removeWatchLater(id)
		return nil
	}
	if a.illustrationFn == nil {
		return errors.New("works are unavailable")
	}
	w, err := a.illustrationFn(ctx, id)
	if err != nil {
		return err
	}
	a.addWatchLater(*w)
	return nil
}

// FollowUser follows or unfollows a user; the MCP follow tool. A private
// follow stays off the user's profile.
func (a *App) FollowUser(ctx context.Context, userID int64, on, private bool) error {
	if on && private {
		if a.followPrivateFn == nil {
			return errors.New("private follows are unavailable")
		}
		return a.followPrivateFn(ctx, userID)
	}
	if a.followFn == nil {
		return errors.New("follows are unavailable")
	}
	return a.followFn(ctx, userID, on)
}

// ControlDownload pauses, resumes or cancels one queued task; the MCP download
// control tool. It never removes a task or deletes a downloaded file.
func (a *App) ControlDownload(_ context.Context, taskID, action string) error {
	if a.downloads == nil {
		return errors.New("downloads are unavailable")
	}
	found := false
	for _, t := range a.downloads.Snapshot() {
		if t.ID == taskID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("unknown download task: %q", taskID)
	}
	switch action {
	case "pause":
		a.downloads.Pause(taskID)
	case "resume":
		a.downloads.Resume(taskID)
	case "cancel":
		a.downloads.Cancel(taskID)
	default:
		return fmt.Errorf("unknown download action: %q", action)
	}
	return nil
}

// AddSubscription follows a user or series for updates; the MCP subscription
// write tool. Only the kinds the check loop can fetch are accepted.
func (a *App) AddSubscription(_ context.Context, kind string, targetID int64, name string) error {
	if a.subsStore == nil {
		return errors.New("subscriptions are unavailable")
	}
	k, err := subscriptionKind(kind)
	if err != nil {
		return err
	}
	id := subscriptionID(k, targetID)
	if a.isSubscribed(k, targetID) {
		return fmt.Errorf("%s %d is already subscribed", kind, targetID)
	}
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("%s %d", kind, targetID)
	}
	if !a.subsStore.Add(subscription.Subscription{
		ID:         id,
		Kind:       k,
		TargetID:   targetID,
		TargetName: name,
	}) {
		return errors.New("subscription was not added: the list is full")
	}
	a.saveSubscriptions()
	return nil
}

// RemoveSubscription stops following a user or series; the MCP subscription
// write tool.
func (a *App) RemoveSubscription(_ context.Context, kind string, targetID int64) error {
	if a.subsStore == nil {
		return errors.New("subscriptions are unavailable")
	}
	k, err := subscriptionKind(kind)
	if err != nil {
		return err
	}
	if !a.subsStore.Remove(subscriptionID(k, targetID)) {
		return fmt.Errorf("%s %d is not subscribed", kind, targetID)
	}
	a.saveSubscriptions()
	return nil
}

// SyncSubscriptions runs one subscription check; the MCP sync tool. It returns
// how many new works the check found and never queues a download.
func (a *App) SyncSubscriptions(ctx context.Context) (int, error) {
	if a.subs == nil {
		return 0, errors.New("subscriptions are unavailable")
	}
	fetched, err := a.subs.CheckDetailed(ctx)
	a.saveSubscriptions()
	if err != nil {
		return len(fetched), err
	}
	return len(fetched), nil
}

// AddComment posts a comment on an illustration; the MCP comment tool. The Go
// client has no novel comment endpoint, so only illustration ids work.
func (a *App) AddComment(ctx context.Context, illustID int64, text string) (*pixiv.Comment, error) {
	if a.addCommentFn == nil {
		return nil, errors.New("comments are unavailable")
	}
	return a.addCommentFn(ctx, illustID, text)
}

// DeleteComment removes one of the signed-in user's comments; the MCP comment
// delete tool.
func (a *App) DeleteComment(ctx context.Context, commentID int64) error {
	if a.deleteCommentFn == nil {
		return errors.New("comments are unavailable")
	}
	return a.deleteCommentFn(ctx, commentID)
}

// SetDownloadMacro validates and saves a new download path macro; the MCP
// macro write tool. The running app picks it up on the main thread.
func (a *App) SetDownloadMacro(ctx context.Context, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("text is required")
	}
	if _, err := download.Parse(text); err != nil {
		return err
	}
	path, err := settingsPath()
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	apply := func() {
		if a.settings == nil {
			done <- errors.New("settings are unavailable")
			return
		}
		a.settings.DownloadMacro = text
		a.publishSettings()
		s := *a.settings
		go func() { done <- Save(path, &s) }()
	}
	if a.win == nil {
		apply()
	} else {
		a.win.Update(apply)
	}
	// A window that stops pumping would otherwise leave the request hanging.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return errors.New("settings save timed out")
	}
}

// subscriptionKind is the stored kind of a subscription the check loop can
// fetch. The tag kind has no endpoint, so it is refused rather than saved as a
// subscription that never checks.
func subscriptionKind(kind string) (subscription.Kind, error) {
	switch subscription.Kind(kind) {
	case subscription.KindUser:
		return subscription.KindUser, nil
	case subscription.KindSeries:
		return subscription.KindSeries, nil
	default:
		return "", errors.New(`kind must be "user" or "series"`)
	}
}
