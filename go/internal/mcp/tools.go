package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

const (
	// defaultLimit is how many works a list tool returns when the call does
	// not say. maxLimit caps a client that asks for more.
	defaultLimit = 10
	maxLimit     = 50
	// defaultListLimit is the default of the newer read tools, which list more
	// than one page of a feed.
	defaultListLimit = 20
)

// property is one field of a tool's input schema.
type property struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Default     any      `json:"default,omitempty"`
}

type schema struct {
	Type                 string              `json:"type"`
	Properties           map[string]property `json:"properties"`
	Required             []string            `json:"required,omitempty"`
	AdditionalProperties bool                `json:"additionalProperties"`
}

// tool is one entry of tools/list.
type tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema schema `json:"inputSchema"`
}

// SavedWork is one work in the local Watch Later list.
type SavedWork struct {
	ID      int64
	Title   string
	Artist  string
	SavedAt time.Time
}

// DownloadedWork is one finished download in the local history.
type DownloadedWork struct {
	IllustID    int64
	Title       string
	Artist      string
	Dest        string
	Page        int
	CompletedAt time.Time
}

// SubscriptionInfo is one followed user, tag or series.
type SubscriptionInfo struct {
	Kind        string
	TargetID    int64
	TargetName  string
	LastChecked time.Time
}

var limitProperty = property{
	Type:        "integer",
	Description: "How many works to return, 1 to 50.",
	Default:     defaultLimit,
}

var idProperty = property{
	Type:        "integer",
	Description: "The work's Pixiv illustration id.",
}

var listLimitProperty = property{
	Type:        "integer",
	Description: "How many items to return, 1 to 50.",
	Default:     defaultListLimit,
}

var wordProperty = property{
	Type:        "string",
	Description: "The search word, for example a name, tag or title.",
}

var userIDProperty = property{
	Type:        "integer",
	Description: "The user's Pixiv id.",
}

var kindProperty = property{
	Type:        "string",
	Description: "Which works to list: illust or manga.",
	Enum:        []string{"illust", "manga"},
	Default:     "illust",
}

var privateProperty = property{
	Type:        "boolean",
	Description: "True lists private bookmarks, false public ones.",
	Default:     false,
}

// toolDefs are the tools this server exposes, in the order a client shows them.
var toolDefs = []tool{
	{
		Name:        "search_illustrations",
		Description: "Search Pixiv works by keyword and return matching illustrations.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"query": {Type: "string", Description: "The search word, for example a tag or a title."},
				"limit": limitProperty,
			},
			Required: []string{"query"},
		},
	},
	{
		Name:        "recommended",
		Description: "List the works Pixiv recommends for the signed-in user.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": limitProperty},
		},
	},
	{
		Name:        "illustration_detail",
		Description: "Read one work's detail: title, artist, tags, pages and stats.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"id": idProperty},
			Required:   []string{"id"},
		},
	},
	{
		Name:        "ranking",
		Description: "List the top works of a ranking period.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"mode": {
					Type:        "string",
					Description: "The ranking period; day when omitted.",
					Enum: []string{
						string(pixiv.Day), string(pixiv.Week), string(pixiv.Month),
						string(pixiv.DayMale), string(pixiv.DayFemale),
						string(pixiv.WeekOriginal), string(pixiv.WeekRookie),
					},
					Default: string(pixiv.Day),
				},
				"limit": limitProperty,
			},
		},
	},
	{
		Name:        "set_bookmark",
		Description: "Add or remove a work's bookmark for the signed-in user.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"id":       idProperty,
				"bookmark": {Type: "boolean", Description: "True adds the bookmark, false removes it.", Default: true},
			},
			Required: []string{"id"},
		},
	},
	{
		Name:        "queue_download",
		Description: "Queue every page of a work for download and report how many pages were queued.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"id": idProperty},
			Required:   []string{"id"},
		},
	},
	{
		Name:        "status",
		Description: "Report the app's state, such as the download queue.",
		InputSchema: schema{Type: "object", Properties: map[string]property{}},
	},
	{
		Name:        "search_users",
		Description: "Search Pixiv users by keyword and return matching profiles.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"word":  wordProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"word"},
		},
	},
	{
		Name:        "user_detail",
		Description: "Read one user's profile: name, account, comment and counts.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"id": userIDProperty},
			Required:   []string{"id"},
		},
	},
	{
		Name:        "user_illusts",
		Description: "List a user's works, illustrations or manga.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"id":    userIDProperty,
				"kind":  kindProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"id"},
		},
	},
	{
		Name:        "user_bookmarks",
		Description: "List the signed-in user's bookmarked works, public or private.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"private": privateProperty,
				"limit":   listLimitProperty,
			},
		},
	},
	{
		Name:        "search_novels",
		Description: "Search Pixiv novels by keyword and return matching novels.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"word":  wordProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"word"},
		},
	},
	{
		Name:        "watch_later",
		Description: "List the works saved for later in Pixeval, newest first.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "download_history",
		Description: "List the completed downloads Pixeval recorded, newest first.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "subscriptions",
		Description: "List the users, tags and series followed for updates.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "related_illustrations",
		Description: "List the works Pixiv relates to one work.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"id":    idProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"id"},
		},
	},
	{
		Name:        "spotlight",
		Description: "List the Pixivision articles featured in Spotlight.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
}

// runTool decodes a tool's arguments and runs it. A tool failure comes back as
// an isError result, so the client can show the reason.
func runTool(ctx context.Context, b Backend, name string, raw json.RawMessage) (callResult, error) {
	switch name {
	case "search_illustrations":
		var args struct {
			Query string `json:"query"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if strings.TrimSpace(args.Query) == "" {
			return callResult{}, errors.New("query is required")
		}
		works, err := b.Search(ctx, args.Query, clampLimit(args.Limit))
		if err != nil {
			return failure(err), nil
		}
		return worksResult(works), nil
	case "recommended":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		works, err := b.Recommended(ctx, clampLimit(args.Limit))
		if err != nil {
			return failure(err), nil
		}
		return worksResult(works), nil
	case "illustration_detail":
		var args struct {
			ID int64 `json:"id"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID <= 0 {
			return callResult{}, errors.New("id is required")
		}
		w, err := b.Detail(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return textResult(workText(w)), nil
	case "ranking":
		var args struct {
			Mode  string `json:"mode"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		works, err := b.Ranking(ctx, args.Mode, clampLimit(args.Limit))
		if err != nil {
			return failure(err), nil
		}
		return worksResult(works), nil
	case "set_bookmark":
		var args struct {
			ID       int64 `json:"id"`
			Bookmark *bool `json:"bookmark"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID <= 0 {
			return callResult{}, errors.New("id is required")
		}
		on := args.Bookmark == nil || *args.Bookmark
		if err := b.SetBookmark(ctx, args.ID, on); err != nil {
			return failure(err), nil
		}
		state := "removed"
		if on {
			state = "added"
		}
		return textResult(fmt.Sprintf("bookmark %s for work %d", state, args.ID)), nil
	case "queue_download":
		var args struct {
			ID int64 `json:"id"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID <= 0 {
			return callResult{}, errors.New("id is required")
		}
		n, err := b.QueueDownload(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return textResult(fmt.Sprintf("queued %d page(s) of work %d", n, args.ID)), nil
	case "status":
		return textResult(b.Status()), nil
	case "search_users":
		var args struct {
			Word  string `json:"word"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if strings.TrimSpace(args.Word) == "" {
			return callResult{}, errors.New("word is required")
		}
		users, err := b.SearchUsers(ctx, args.Word)
		if err != nil {
			return failure(err), nil
		}
		return usersResult(limitItems(users, clampListLimit(args.Limit))), nil
	case "user_detail":
		var args struct {
			ID int64 `json:"id"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID <= 0 {
			return callResult{}, errors.New("id is required")
		}
		u, err := b.UserDetail(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return textResult(userDetailText(u)), nil
	case "user_illusts":
		var args struct {
			ID    int64  `json:"id"`
			Kind  string `json:"kind"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID <= 0 {
			return callResult{}, errors.New("id is required")
		}
		kind := args.Kind
		if kind == "" {
			kind = "illust"
		}
		if kind != "illust" && kind != "manga" {
			return callResult{}, errors.New(`kind must be "illust" or "manga"`)
		}
		works, err := b.UserIllusts(ctx, args.ID, kind)
		if err != nil {
			return failure(err), nil
		}
		return worksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "user_bookmarks":
		var args struct {
			Private bool `json:"private"`
			Limit   int  `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		// Zero asks the backend for the signed-in account.
		works, err := b.UserBookmarks(ctx, 0, args.Private)
		if err != nil {
			return failure(err), nil
		}
		return worksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "search_novels":
		var args struct {
			Word  string `json:"word"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if strings.TrimSpace(args.Word) == "" {
			return callResult{}, errors.New("word is required")
		}
		novels, err := b.SearchNovels(ctx, args.Word)
		if err != nil {
			return failure(err), nil
		}
		return novelsResult(limitItems(novels, clampListLimit(args.Limit))), nil
	case "watch_later":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		works, err := b.WatchLater(ctx)
		if err != nil {
			return failure(err), nil
		}
		return savedWorksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "download_history":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		entries, err := b.DownloadHistory(ctx)
		if err != nil {
			return failure(err), nil
		}
		return downloadsResult(limitItems(entries, clampListLimit(args.Limit))), nil
	case "subscriptions":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		subs, err := b.Subscriptions(ctx)
		if err != nil {
			return failure(err), nil
		}
		return subscriptionsResult(limitItems(subs, clampListLimit(args.Limit))), nil
	case "related_illustrations":
		var args struct {
			ID    int64 `json:"id"`
			Limit int   `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID <= 0 {
			return callResult{}, errors.New("id is required")
		}
		works, err := b.RelatedIllusts(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return worksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "spotlight":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		articles, err := b.Spotlight(ctx)
		if err != nil {
			return failure(err), nil
		}
		return articlesResult(limitItems(articles, clampListLimit(args.Limit))), nil
	default:
		return callResult{}, fmt.Errorf("unknown tool: %q", name)
	}
}

// decodeArgs unmarshals a tool's arguments, treating an absent object as empty.
func decodeArgs(raw json.RawMessage, out any) error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("bad arguments: %w", err)
	}
	return nil
}

// clampLimit is the limit a list tool uses: the default when unset, capped at
// maxLimit.
func clampLimit(n int) int {
	switch {
	case n <= 0:
		return defaultLimit
	case n > maxLimit:
		return maxLimit
	default:
		return n
	}
}

// clampListLimit is clampLimit for the newer read tools, whose default is
// defaultListLimit.
func clampListLimit(n int) int {
	switch {
	case n <= 0:
		return defaultListLimit
	case n > maxLimit:
		return maxLimit
	default:
		return n
	}
}

// limitItems keeps at most limit items, so a tool never returns more than its
// schema promises.
func limitItems[T any](items []T, limit int) []T {
	if len(items) > limit {
		return items[:limit]
	}
	return items
}

func textResult(s string) callResult {
	return callResult{Content: []content{{Type: "text", Text: s}}}
}

func failure(err error) callResult {
	return callResult{Content: []content{{Type: "text", Text: err.Error()}}, IsError: true}
}

func worksResult(works []pixiv.Illustration) callResult {
	if len(works) == 0 {
		return textResult("no works found")
	}
	lines := make([]string, 0, len(works))
	for i := range works {
		lines = append(lines, workText(&works[i]))
	}
	return textResult(strings.Join(lines, "\n\n"))
}

// listResult formats a list of items, or says there are none.
func listResult[T any](items []T, empty string, format func(*T) string) callResult {
	if len(items) == 0 {
		return textResult(empty)
	}
	lines := make([]string, 0, len(items))
	for i := range items {
		lines = append(lines, format(&items[i]))
	}
	return textResult(strings.Join(lines, "\n\n"))
}

func usersResult(users []pixiv.UserPreview) callResult {
	return listResult(users, "no users found", func(u *pixiv.UserPreview) string {
		return userText(&u.User)
	})
}

func novelsResult(novels []pixiv.Novel) callResult {
	return listResult(novels, "no novels found", novelText)
}

func savedWorksResult(works []SavedWork) callResult {
	return listResult(works, "nothing saved for later", savedWorkText)
}

func downloadsResult(entries []DownloadedWork) callResult {
	return listResult(entries, "no downloads recorded", downloadText)
}

func subscriptionsResult(subs []SubscriptionInfo) callResult {
	return listResult(subs, "no subscriptions", subscriptionText)
}

func articlesResult(articles []pixiv.SpotlightArticle) callResult {
	return listResult(articles, "no articles found", articleText)
}

// userText is one user a client can read and open: the profile line and its
// page URL.
func userText(u *pixiv.User) string {
	if u == nil {
		return "no user found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s", u.ID, u.Name)
	if u.Account != "" {
		fmt.Fprintf(&b, " (@%s)", u.Account)
	}
	b.WriteString("\n")
	if c := clip(u.Comment, 300); c != "" {
		fmt.Fprintf(&b, "%s\n", c)
	}
	fmt.Fprintf(&b, "https://www.pixiv.net/users/%d", u.ID)
	return b.String()
}

// userDetailText is one user's profile with the counts the detail endpoint
// adds.
func userDetailText(d *pixiv.UserDetail) string {
	if d == nil {
		return "no user found"
	}
	var b strings.Builder
	b.WriteString(userText(&d.User))
	p := d.Profile
	fmt.Fprintf(&b, "\n%d works, %d novels, %d public bookmarks",
		p.TotalIllusts+p.TotalManga, p.TotalNovels, p.TotalIllustBookmarksPublic)
	if p.Webpage != "" {
		fmt.Fprintf(&b, "\n%s", p.Webpage)
	}
	return b.String()
}

// novelText is one novel a client can read and open.
func novelText(n *pixiv.Novel) string {
	if n == nil {
		return "no novel found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s — %s\n", n.ID, n.Title, n.User.Name)
	fmt.Fprintf(&b, "%d bookmarks, %d page(s)\n", n.TotalBookmarks, n.PageCount)
	fmt.Fprintf(&b, "https://www.pixiv.net/novel/show.php?id=%d", n.ID)
	return b.String()
}

// savedWorkText is one work saved for later.
func savedWorkText(w *SavedWork) string {
	if w == nil {
		return "no work found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s — %s\n", w.ID, w.Title, w.Artist)
	if !w.SavedAt.IsZero() {
		fmt.Fprintf(&b, "saved %s\n", w.SavedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "https://www.pixiv.net/artworks/%d", w.ID)
	return b.String()
}

func downloadText(d *DownloadedWork) string {
	if d == nil {
		return "no download found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s — %s\n", d.IllustID, d.Title, d.Artist)
	fmt.Fprintf(&b, "page %d → %s\n", d.Page+1, d.Dest)
	if !d.CompletedAt.IsZero() {
		fmt.Fprintf(&b, "completed %s\n", d.CompletedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "https://www.pixiv.net/artworks/%d", d.IllustID)
	return b.String()
}

func subscriptionText(s *SubscriptionInfo) string {
	if s == nil {
		return "no subscription found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s (#%d)", s.Kind, s.TargetName, s.TargetID)
	if !s.LastChecked.IsZero() {
		fmt.Fprintf(&b, "\nlast checked %s", s.LastChecked.Format(time.RFC3339))
	}
	return b.String()
}

func articleText(a *pixiv.SpotlightArticle) string {
	if a == nil {
		return "no article found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", a.Title)
	if a.PublishDate != "" {
		fmt.Fprintf(&b, "%s\n", a.PublishDate)
	}
	b.WriteString(a.ArticleURL)
	return b.String()
}

// clip folds s to one line and keeps at most n runes, so a long comment does
// not flood a tool result.
func clip(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// workText is one work a client can read and act on: a line of facts and its
// page URL.
func workText(w *pixiv.Illustration) string {
	if w == nil {
		return "no work found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s — %s\n", w.ID, w.Title, w.User.Name)
	fmt.Fprintf(&b, "%d bookmarks, %d views, %dx%d, %d page(s)\n",
		w.TotalBookmarks, w.TotalView, w.Width, w.Height, pageCount(w))
	if len(w.Tags) > 0 {
		tags := make([]string, 0, len(w.Tags))
		for _, t := range w.Tags {
			tags = append(tags, t.Name)
		}
		fmt.Fprintf(&b, "tags: %s\n", strings.Join(tags, ", "))
	}
	fmt.Fprintf(&b, "https://www.pixiv.net/artworks/%d", w.ID)
	return b.String()
}

// pageCount is the number of pages a work has, at least one.
func pageCount(w *pixiv.Illustration) int {
	if w.PageCount < 1 {
		return 1
	}
	return w.PageCount
}
