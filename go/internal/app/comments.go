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

// commentAvatar is the side of one comment's avatar.
const commentAvatar = 28

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
	// sentDraft is the text a send posted, written off the main thread. The
	// panel clears the composer on the next frame, where draft is its own.
	sentDraft string
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

// toggleComments shows or hides the comment section, loading the first page
// the first time it opens.
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
	if a.session == nil {
		// Signed out, the card draws no composer; this guards a stale send.
		a.notice = a.t("comments.login")
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
			if err == nil {
				st.sentDraft = draft
			}
			st.mu.Unlock()
			if err != nil {
				a.notice = a.tf("comments.sendFailed", err.Error())
				return
			}
			a.loadComments(st, true)
		})
	}()
}

// takeSentDraft returns the text of the send that finished, once.
func (st *commentState) takeSentDraft() (string, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	sent := st.sentDraft
	st.sentDraft = ""
	return sent, sent != ""
}

func (st *commentState) retry() {
	st.mu.Lock()
	st.err = ""
	st.mu.Unlock()
}

// commentsSection draws the viewer's comments inside the info card: the title
// and count, the list, and the composer. The card's padding frames it, so it
// sets no width or outer border of its own.
func (a *App) commentsSection(c *ui.Context, t tokens, st *commentState) {
	// The composer is edited on the main thread only: a send leaves the text
	// it posted here, and this frame clears it if it is still what was sent.
	if sent, ok := st.takeSentDraft(); ok && st.draft == sent {
		st.draft, st.replyTo = "", nil
	}
	v := st.snapshot()
	ui.Column(c).Gap(t.SpaceM).Padding(t.SpaceXL, 0, 0, 0).
		BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		ui.Row(c).Gap(t.SpaceXS).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.t("comments.title")).FontSize(t.TextMicro).LetterSpacing(1.2).
				TextColor(t.TextMuted)
			if len(v.items) > 0 {
				ui.Text(c, detailCount(len(v.items))).FontSize(t.TextMicro).
					TextColor(t.TextMuted)
			}
		})
		if v.err != "" {
			ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
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
			a.commentItem(c, t, st, &v.items[i])
		}
		if v.loading {
			ui.Text(c, a.t("common.loading")).FontSize(t.TextTiny).TextColor(t.TextMuted)
		}
		if v.hasMore && !v.loading && v.err == "" {
			if ui.Button(c, a.t("comments.loadMore")).Clicked() {
				a.loadComments(st, false)
			}
		}
		if reply := st.replyTo; reply != nil {
			ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tf("comments.replyingTo", reply.User.Name)).
					FontSize(t.TextTiny).TextColor(t.TextMuted).Grow(1).MaxLines(1)
				if ui.Button(c, a.t("comments.cancel")).Clicked() {
					st.replyTo = nil
				}
			})
		}
		ui.Row(c).Gap(t.SpaceS).Children(func() {
			ui.TextInput(c, &st.draft).Grow(1).
				Placeholder(a.t("comments.placeholder")).Label(a.t("comments.placeholder"))
			if ui.Button(c, a.t("comments.send")).Disabled(v.sending).Clicked() {
				a.submitComment(st)
			}
		})
	})
}

// commentItem draws one comment: its avatar, author, age, text and reply
// button.
func (a *App) commentItem(c *ui.Context, t tokens, st *commentState, cm *pixiv.Comment) {
	ui.Row(c).Gap(t.SpaceS).AlignItems(ui.Start).
		Label(a.tf("a11y.comment", cm.User.Name)).Children(func() {
		avatar := ui.Box(c).Width(commentAvatar).Height(commentAvatar).Shrink(0).
			Radius(t.RadiusFull).Background(t.SurfaceAlt).Clip().
			Label(a.tf("a11y.avatar", cm.User.Name))
		url := cm.User.ProfileImageUrls.Px50x50
		if url != "" && !st.avatarFailedURL(url) {
			avatar.Children(func() {
				if bmp := a.images.Thumb(url, commentAvatar, commentAvatar, a.commentAvatarReady(st, url)); bmp != nil {
					ui.Image(c, bmp).Fill()
				}
			})
		}
		ui.Column(c).Grow(1).Gap(2).MinWidth(0).Children(func() {
			ui.Row(c).Gap(t.SpaceXS).Children(func() {
				ui.Text(c, cm.User.Name).FontSize(t.TextSmall).FontWeight(600).
					TextColor(t.Text).MaxLines(1).Grow(1)
				ui.Text(c, a.commentTime(cm.CreateDate)).FontSize(t.TextMicro).
					TextColor(t.TextMuted)
			})
			ui.Text(c, cm.Comment).FontSize(t.TextSmall).LineHeight(1.55).
				TextColor(t.Text).MaxLines(5)
			rb := ui.ButtonBase(c).Height(22).Radius(t.RadiusXS).Padding(0).
				Label(a.t("comments.reply"))
			rf := t.TextMuted
			if rb.Hovered() {
				rf = t.Accent
			}
			if rb.Clicked() {
				st.replyTo = cm
			}
			rb.Children(func() {
				ui.Text(c, a.t("comments.reply")).FontSize(t.TextMicro).TextColor(rf)
			})
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
