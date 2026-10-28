package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// The comment panel's width in DIPs, and the side of one comment's avatar.
const (
	commentsWidth = 360
	commentAvatar = 28
)

// commentState is one work's comment panel: the comments loaded so far, the
// offset of the next page, the composer and the comment a reply answers.
// draft and replyTo belong to the composer and are only touched on the main
// thread, so they need no lock; the rest is written by the load goroutine and
// read by the view through snapshot, under mu.
type commentState struct {
	workID int64

	draft   string
	replyTo *pixiv.Comment

	mu      sync.Mutex
	items   []pixiv.Comment
	next    string
	offset  int
	loading bool
	sending bool
	err     string
	// started is set once a page loaded, so an empty next ends the list
	// instead of opening the panel on the first page again.
	started bool
	// gen identifies the list a load belongs to: a reload starts the next one
	// and drops the results of the old.
	gen int
	// avatarFailed holds the avatars that did not load, so a broken URL is not
	// asked for again every frame.
	avatarFailed map[string]bool
}

// commentsView is what the panel draws.
type commentsView struct {
	items   []pixiv.Comment
	loading bool
	sending bool
	hasMore bool
	err     string
}

func (st *commentState) snapshot() commentsView {
	st.mu.Lock()
	defer st.mu.Unlock()
	return commentsView{
		items:   st.items,
		loading: st.loading,
		sending: st.sending,
		hasMore: st.next != "",
		err:     st.err,
	}
}

func (st *commentState) avatarFailedURL(url string) bool {
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.avatarFailed[url]
}

// commentsButton toggles the comment panel from the viewer's toolbar.
func (a *App) commentsButton(c *ui.Context) {
	if ui.Button(c, a.t("comments.title")).Clicked() {
		a.toggleComments()
	}
}

// toggleComments shows or hides the comment panel, loading the first page the
// first time it opens.
func (a *App) toggleComments() {
	if a.viewer == nil {
		return
	}
	a.commentsOpen = !a.commentsOpen
	st := a.comments
	if !a.commentsOpen || st == nil {
		return
	}
	st.mu.Lock()
	idle := !st.started && !st.loading && st.err == ""
	st.mu.Unlock()
	if idle {
		a.loadComments(st, false)
	}
}

// loadComments fetches one page of comments off the main thread and appends
// it. A reset drops the loaded pages and starts again from the first.
func (a *App) loadComments(st *commentState, reset bool) {
	if st == nil {
		return
	}
	st.mu.Lock()
	if reset {
		st.gen++
		st.items, st.next, st.offset = nil, "", 0
		st.started, st.err, st.loading = false, "", false
	}
	if st.loading || a.fetchCommentPage == nil {
		st.mu.Unlock()
		return
	}
	st.loading = true
	gen, workID, offset := st.gen, st.workID, st.offset
	st.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), browseTimeout)
		defer cancel()
		page, err := a.fetchCommentPage(ctx, workID, offset)
		a.update(func() {
			st.mu.Lock()
			defer st.mu.Unlock()
			if gen != st.gen {
				// A reload replaced this list; its page is not wanted.
				return
			}
			st.loading = false
			switch {
			case errors.Is(err, pixiv.ErrNoMore):
				st.next = ""
				st.started = true
			case err != nil:
				st.err = err.Error()
			default:
				st.started = true
				st.items = append(st.items, page.Items...)
				st.next = page.NextURL
				st.offset = len(st.items)
			}
		})
	}()
}

// submitComment posts the draft off the main thread. A reply goes as the
// author's name in front of the text, because the API only takes a comment
// field and has no reply target. On success the composer clears and the first
// page loads again, so the new comment is shown in place.
func (a *App) submitComment(st *commentState) {
	if st == nil {
		return
	}

	draft := st.draft
	text := strings.TrimSpace(draft)
	if text == "" || a.addCommentFn == nil {
		return
	}

	st.mu.Lock()
	if st.sending {
		st.mu.Unlock()
		return
	}
	st.sending = true
	workID := st.workID
	st.mu.Unlock()

	if st.replyTo != nil {
		text = "@" + st.replyTo.User.Name + " " + text
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), bookmarkTimeout)
		defer cancel()
		_, err := a.addCommentFn(ctx, workID, text)
		a.update(func() {
			st.mu.Lock()
			st.sending = false
			st.mu.Unlock()
			if err != nil {
				a.notice = a.tf("comments.sendFailed", err.Error())
				return
			}
			// Clear only the text that was sent: anything typed during the
			// send stays in the composer.
			if st.draft == draft {
				st.draft, st.replyTo = "", nil
			}
			a.loadComments(st, true)
		})
	}()
}

func (st *commentState) retry() {
	st.mu.Lock()
	st.err = ""
	st.mu.Unlock()
}

// commentsPanel draws the comment panel beside the work: its title, the
// scrollable list, and the composer.
func (a *App) commentsPanel(c *ui.Context, st *commentState) {
	t := c.Theme()
	v := st.snapshot()
	ui.Column(c).Width(commentsWidth).Shrink(0).FillHeight().
		Background(t.Surface).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() {
		ui.Row(c).Padding(12).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.t("comments.title")).FontSize(16).Bold().Grow(1)
		})
		ui.Scroll(c).Grow(1).Children(func() {
			ui.Column(c).Padding(12).Gap(12).Children(func() {
				if v.err != "" {
					ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
						ui.Text(c, a.tf("common.loadFailed", v.err)).TextColor(t.Danger).Grow(1)
						if ui.Button(c, a.t("common.retry")).Clicked() {
							st.retry()
							a.loadComments(st, false)
						}
					})
				}
				if len(v.items) == 0 && !v.loading && !v.hasMore && v.err == "" {
					ui.Text(c, a.t("comments.empty")).TextColor(t.TextMuted)
				}
				for i := range v.items {
					a.commentItem(c, st, &v.items[i])
				}
				if v.loading {
					ui.Text(c, a.t("common.loading")).FontSize(12).TextColor(t.TextMuted)
				}
				if v.hasMore && !v.loading && v.err == "" {
					if ui.Button(c, a.t("comments.loadMore")).Clicked() {
						a.loadComments(st, false)
					}
				}
			})
		})
		if reply := st.replyTo; reply != nil {
			ui.Row(c).Gap(8).Padding(12, 0, 12, 0).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tf("comments.replyingTo", reply.User.Name)).
					FontSize(12).TextColor(t.TextMuted).Grow(1).MaxLines(1)
				if ui.Button(c, a.t("comments.cancel")).Clicked() {
					st.replyTo = nil
				}
			})
		}
		ui.Row(c).Gap(8).Padding(12).Children(func() {
			ui.TextInput(c, &st.draft).Grow(1).Placeholder(a.t("comments.placeholder"))
			if ui.Button(c, a.t("comments.send")).Disabled(v.sending).Clicked() {
				a.submitComment(st)
			}
		})
	})
}

// commentItem draws one comment: its avatar, author, age, text and reply
// button.
func (a *App) commentItem(c *ui.Context, st *commentState, cm *pixiv.Comment) {
	t := c.Theme()
	ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
		avatar := ui.Box(c).Width(commentAvatar).Height(commentAvatar).Shrink(0).
			Radius(commentAvatar / 2).Background(t.Background).Clip()
		url := cm.User.ProfileImageUrls.Px50x50
		if url != "" && !st.avatarFailedURL(url) {
			avatar.Children(func() {
				if bmp := a.images.Thumb(url, commentAvatar, commentAvatar, a.commentAvatarReady(st, url)); bmp != nil {
					ui.Image(c, bmp).Fill()
				}
			})
		}
		ui.Column(c).Grow(1).Gap(2).Children(func() {
			ui.Row(c).Gap(6).Children(func() {
				ui.Text(c, cm.User.Name).Bold().MaxLines(1).Grow(1)
				ui.Text(c, a.commentTime(cm.CreateDate)).FontSize(11).TextColor(t.TextMuted)
			})
			ui.Text(c, cm.Comment).MaxLines(4)
			if ui.Button(c, a.t("comments.reply")).Clicked() {
				st.replyTo = cm
			}
		})
	})
}

// commentAvatarReady returns the callback an avatar load calls: a probe that
// finds nothing means the load failed, and the avatar stays a block from then
// on rather than loading again every frame.
func (a *App) commentAvatarReady(st *commentState, url string) func() {
	return func() {
		if a.images.Thumb(url, commentAvatar, commentAvatar, nil) == nil {
			st.mu.Lock()
			if st.avatarFailed == nil {
				st.avatarFailed = map[string]bool{}
			}
			st.avatarFailed[url] = true
			st.mu.Unlock()
		}
		a.update(func() {})
	}
}

// commentTime is how long ago the comment was posted, in the coarsest unit
// that fits.
func (a *App) commentTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return a.t("comments.justNow")
	case d < time.Hour:
		return a.tf("comments.minutesAgo", int(d.Minutes()))
	case d < 24*time.Hour:
		return a.tf("comments.hoursAgo", int(d.Hours()))
	default:
		return a.tf("comments.daysAgo", int(d.Hours()/24))
	}
}
