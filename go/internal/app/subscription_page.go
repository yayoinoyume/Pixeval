package app

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
	"github.com/yayoinoyume/pixeval-go/internal/subscription"
)

// subscriptionInterval is how often the background check looks for new works.
const subscriptionInterval = 6 * time.Hour

// pixivFetcher answers a subscription with the newest page of its works.
type pixivFetcher struct{ client *pixiv.Client }

// Fetch returns the first page of the followed target's works. A kind without
// an endpoint is an error, which the check reports by skipping it.
func (f pixivFetcher) Fetch(ctx context.Context, sub subscription.Subscription) ([]pixiv.Illustration, error) {
	switch sub.Kind {
	case subscription.KindUser:
		page, err := f.client.UserIllusts(ctx, sub.TargetID, "illust")
		if err != nil {
			return nil, err
		}
		return page.Items, nil
	case subscription.KindSeries:
		page, err := f.client.SeriesWorks(ctx, sub.TargetID)
		if err != nil {
			return nil, err
		}
		return page.Illusts, nil
	default:
		return nil, fmt.Errorf("subscription: kind %q has no endpoint", sub.Kind)
	}
}

// subscriptionsPath is where the followed targets live: the user's config
// directory, like the settings.
func subscriptionsPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pixeval-go", "subscriptions.json"), nil
}

// loadSubscriptions reads the saved list into st. A missing file is an empty
// list, which is the first run.
func loadSubscriptions(st *subscription.Store) error {
	path, err := subscriptionsPath()
	if err != nil {
		return err
	}
	if err := st.Load(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// saveSubscriptions writes the list to the config directory.
func (a *App) saveSubscriptions() {
	path, err := subscriptionsPath()
	if err != nil {
		log.Printf("subscriptions: %v", err)
		return
	}
	if err := a.subsStore.Save(path); err != nil {
		log.Printf("subscriptions: save: %v", err)
	}
}

// subscriptionID is the stored ID of a followed target; the kind and the ID
// together are unique.
func subscriptionID(kind subscription.Kind, targetID int64) string {
	return fmt.Sprintf("%s-%d", kind, targetID)
}

// isSubscribed reports whether the target is already followed.
func (a *App) isSubscribed(kind subscription.Kind, targetID int64) bool {
	if a.subsStore == nil {
		return false
	}
	id := subscriptionID(kind, targetID)
	for _, sub := range a.subsStore.List() {
		if sub.ID == id {
			return true
		}
	}
	return false
}

// toggleSubscription adds the target to the list, or removes it when it is
// already there.
func (a *App) toggleSubscription(kind subscription.Kind, targetID int64, name string) {
	if a.subsStore == nil || targetID <= 0 {
		return
	}
	id := subscriptionID(kind, targetID)
	if a.subsStore.Remove(id) {
		a.saveSubscriptions()
		return
	}
	// LastChecked stays zero, so the first check reports the newest page as new
	// works rather than waiting for the next post.
	if !a.subsStore.Add(subscription.Subscription{
		ID:         id,
		Kind:       kind,
		TargetID:   targetID,
		TargetName: name,
	}) {
		a.notice = a.t("subs.addFailed")
		a.update(func() {})
		return
	}
	a.saveSubscriptions()
}

// subscriptionButton draws the follow-updates button for a user or a series.
func (a *App) subscriptionButton(c *ui.Context, kind subscription.Kind, targetID int64, name string) {
	if a.subsStore == nil || targetID <= 0 {
		return
	}
	t := c.Theme()
	label := a.t("subs.add")
	b := ui.Button(c, label)
	if a.isSubscribed(kind, targetID) {
		b = ui.Button(c, a.t("subs.added")).TextColor(t.Accent)
	}
	if b.Clicked() {
		a.toggleSubscription(kind, targetID, name)
	}
}

// subsSnapshot is the new works, whether a check runs, and the result message
// to toast once; the message is cleared here.
func (a *App) subsSnapshot() ([]pixiv.Illustration, bool, string) {
	a.subsMu.Lock()
	defer a.subsMu.Unlock()
	msg := a.subsMsg
	a.subsMsg = ""
	return append([]pixiv.Illustration(nil), a.newWorks...), a.subsChecking, msg
}

// checkSubscriptions runs one check off the main thread and shows its result.
func (a *App) checkSubscriptions() {
	if a.subs == nil || a.subsStore == nil {
		return
	}
	a.subsMu.Lock()
	if a.subsChecking {
		a.subsMu.Unlock()
		return
	}
	a.subsChecking = true
	a.subsMu.Unlock()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		works, err := a.subs.Check(ctx)
		// A check advances LastChecked, so the next start must see it.
		a.saveSubscriptions()
		a.update(func() {
			// The result lands under the lock, so the page reads the works and
			// the message together.
			a.subsMu.Lock()
			a.subsChecking = false
			a.newWorks = append(a.newWorks, works...)
			if err != nil {
				a.subsMsg = a.tf("subs.checkFailed", err.Error())
			} else {
				a.subsMsg = a.tf("subs.checkDone", len(works))
			}
			a.subsMu.Unlock()
		})
	}()
}

// downloadNewWorks queues every new work.
func (a *App) downloadNewWorks(works []pixiv.Illustration) {
	pages := 0
	for _, w := range works {
		n, ok := a.downloadWork(w)
		if !ok {
			a.notice = a.t("viewer.noDownloadDir")
			a.update(func() {})
			return
		}
		pages += n
	}
	a.notice = a.tf("subs.queued", pages)
	a.update(func() {})
}

// subscriptionsPage draws the followed targets and the works the last check
// found.
func (a *App) subscriptionsPage(c *ui.Context) {
	t := c.Theme()
	var subs []subscription.Subscription
	if a.subsStore != nil {
		subs = a.subsStore.List()
	}
	works, checking, msg := a.subsSnapshot()
	if msg != "" {
		c.Toast(msg)
	}
	ui.Column(c).Grow(1).FillHeight().Children(func() {
		ui.Row(c).Gap(8).Padding(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tf("subs.count", len(subs))).Grow(1).TextColor(t.TextMuted)
			if ui.Button(c, a.t("subs.check")).Disabled(checking).Clicked() {
				a.checkSubscriptions()
			}
			if ui.Button(c, a.t("subs.downloadAll")).Disabled(len(works) == 0).Clicked() {
				a.downloadNewWorks(works)
			}
		})
		if len(subs) == 0 && len(works) == 0 {
			ui.Column(c).Grow(1).Center().Gap(8).Children(func() {
				ui.Text(c, a.t("subs.empty")).TextColor(t.TextMuted)
				ui.Text(c, a.t("subs.emptyHint")).FontSize(12).TextColor(t.TextMuted)
			})
			return
		}
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(12).Gap(4).Children(func() {
				for _, sub := range subs {
					a.subscriptionRow(c, sub)
				}
				a.newWorksList(c, works)
			})
		})
	})
}

// subscriptionRow draws one subscription: what it follows, when it was last
// checked, and the button removing it.
func (a *App) subscriptionRow(c *ui.Context, sub subscription.Subscription) {
	t := c.Theme()
	kind := a.t("subs.kindUser")
	if sub.Kind == subscription.KindSeries {
		kind = a.t("subs.kindSeries")
	}
	ui.Row(c).Gap(12).Padding(10, 12).AlignItems(ui.Center).
		Label(a.tf("a11y.subscription", sub.TargetName)).Children(func() {
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Text(c, sub.TargetName).MaxLines(1)
			ui.Text(c, a.tf("subs.rowDetail", kind, a.checkedLabel(sub))).FontSize(12).TextColor(t.TextMuted)
		})
		if ui.Button(c, a.t("subs.remove")).Clicked() {
			a.subsStore.Remove(sub.ID)
			a.saveSubscriptions()
		}
	})
}

// checkedLabel is when a subscription was last checked, or that it never was.
func (a *App) checkedLabel(sub subscription.Subscription) string {
	if sub.LastChecked.IsZero() {
		return a.t("subs.never")
	}
	return sub.LastChecked.Local().Format("2006-01-02 15:04")
}

// newWorksList draws the works the last checks found; a click opens one in the
// viewer.
func (a *App) newWorksList(c *ui.Context, works []pixiv.Illustration) {
	if len(works) == 0 {
		return
	}
	t := c.Theme()
	ui.Column(c).Padding(0, 0, 12, 0).Gap(4).Children(func() {
		ui.Text(c, a.tf("subs.newWorks", len(works))).FontSize(14).Bold().Padding(0, 12)
		for i := range works {
			work := works[i]
			ui.Column(c).Gap(2).Padding(6, 12).Focusable().
				Label(a.tf("a11y.card", work.Title, work.User.Name)).
				Cursor(ui.CursorPointer).OnClick(func() {
				a.openViewer(&work, 0)
			}).Children(func() {
				ui.Text(c, work.Title).MaxLines(1)
				ui.Text(c, work.User.Name).FontSize(12).TextColor(t.TextMuted).MaxLines(1)
			})
		}
	})
}
