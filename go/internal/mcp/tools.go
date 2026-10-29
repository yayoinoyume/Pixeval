package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/filter"
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
	// maxBatch caps the ids a batch read tool loads in one call.
	maxBatch = 100
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

// HistoryWork is one work the user opened, and when.
type HistoryWork struct {
	ID       int64
	Title    string
	Artist   string
	OpenedAt time.Time
}

// DownloadTask is one task in the local download queue.
type DownloadTask struct {
	ID       string
	IllustID int64
	Title    string
	Artist   string
	Page     int
	State    string
	Bytes    int64
	Total    int64
	Err      string
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

var novelIDProperty = property{
	Type:        "integer",
	Description: "The novel's Pixiv id.",
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

var seriesKindProperty = property{
	Type:        "string",
	Description: "Which series to list: manga or novel.",
	Enum:        []string{"manga", "novel"},
	Default:     "manga",
}

var optionalUserIDProperty = property{
	Type:        "integer",
	Description: "The user's Pixiv id; omit for the signed-in account.",
}

var idsProperty = property{
	Type:        "array",
	Description: "Pixiv ids to load; at most 100 are used.",
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
		Name:        "novel_comments",
		Description: "Read comments on a novel, newest first.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"id":    novelIDProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"id"},
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
	{
		Name:        "new_works",
		Description: "List the newest works posted across Pixiv, illustrations or manga.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"kind":  kindProperty,
				"limit": listLimitProperty,
			},
		},
	},
	{
		Name:        "following_works",
		Description: "List the public works of the users the signed-in account follows.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "my_pixiv_works",
		Description: "List the works of the users in the signed-in account's MyPixiv, illustrations or manga.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"kind":  kindProperty,
				"limit": listLimitProperty,
			},
		},
	},
	{
		Name:        "following_users",
		Description: "List the users a Pixiv user follows publicly, the signed-in account when no id is given.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"id":    optionalUserIDProperty,
				"limit": listLimitProperty,
			},
		},
	},
	{
		Name:        "followers",
		Description: "List the users who follow the signed-in account.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "my_pixiv_users",
		Description: "List the users a Pixiv user added to MyPixiv.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"id":    userIDProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"id"},
		},
	},
	{
		Name:        "related_users",
		Description: "List the users Pixiv lists beside one user.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"id":    userIDProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"id"},
		},
	},
	{
		Name:        "series_watchlist",
		Description: "List the manga or novel series the signed-in account follows.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"kind":  seriesKindProperty,
				"limit": listLimitProperty,
			},
		},
	},
	{
		Name:        "trending_tags",
		Description: "List the illustration tags Pixiv lists as trending now.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "settings_summary",
		Description: "Read the app's saved preferences, without secrets such as the MCP token or API key.",
		InputSchema: schema{Type: "object", Properties: map[string]property{}},
	},
	{
		Name:        "capabilities",
		Description: "List this server's protocol, authentication and tool capabilities.",
		InputSchema: schema{Type: "object", Properties: map[string]property{}},
	},
	{
		Name:        "help",
		Description: "Read the MCP help: every tool, or one tool's arguments and description.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"tool": {Type: "string", Description: "Optional tool name; omit to list every tool."},
			},
		},
	},
	{
		Name:        "download_macro",
		Description: "Read the download path macro the app currently uses.",
		InputSchema: schema{Type: "object", Properties: map[string]property{}},
	},
	{
		Name:        "analyze_download_macro",
		Description: "Parse a download path macro and report whether it is valid, with a rendered sample.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"text": {Type: "string", Description: "The download macro text to analyze."},
			},
			Required: []string{"text"},
		},
	},
	{
		Name:        "history",
		Description: "List the works opened in Pixeval, newest first.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "extensions",
		Description: "Report the extension host state; the Go version has no extension host.",
		InputSchema: schema{Type: "object", Properties: map[string]property{}},
	},
	{
		Name:        "novel_content",
		Description: "Read a novel's body text.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"id": novelIDProperty},
			Required:   []string{"id"},
		},
	},
	{
		Name:        "recommended_users",
		Description: "List the users Pixiv recommends to the signed-in account.",
		InputSchema: schema{
			Type:       "object",
			Properties: map[string]property{"limit": listLimitProperty},
		},
	},
	{
		Name:        "comments",
		Description: "Read the comments on one illustration, newest first.",
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
		Name:        "works",
		Description: "Read several works by id, illustrations or manga.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"ids":   idsProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"ids"},
		},
	},
	{
		Name:        "users",
		Description: "Read several users' profiles by id.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"ids":   idsProperty,
				"limit": listLimitProperty,
			},
			Required: []string{"ids"},
		},
	},
	{
		Name:        "download_tasks",
		Description: "List the tasks in the local download queue, with their state and progress.",
		InputSchema: schema{Type: "object", Properties: map[string]property{}},
	},
	{
		Name:        "analyze_work_filter",
		Description: "Parse a work filter expression and report whether it is valid.",
		InputSchema: schema{
			Type: "object",
			Properties: map[string]property{
				"text": {Type: "string", Description: "The work filter expression to analyze."},
			},
			Required: []string{"text"},
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
	case "novel_comments":
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
		comments, err := b.NovelComments(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return commentsResult(limitItems(comments, clampListLimit(args.Limit))), nil
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
	case "new_works":
		var args struct {
			Kind  string `json:"kind"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		kind, err := workKind(args.Kind)
		if err != nil {
			return callResult{}, err
		}
		works, err := b.NewWorks(ctx, kind)
		if err != nil {
			return failure(err), nil
		}
		return worksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "following_works":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		works, err := b.FollowingWorks(ctx)
		if err != nil {
			return failure(err), nil
		}
		return worksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "my_pixiv_works":
		var args struct {
			Kind  string `json:"kind"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		kind, err := workKind(args.Kind)
		if err != nil {
			return callResult{}, err
		}
		works, err := b.MyPixivWorks(ctx, kind)
		if err != nil {
			return failure(err), nil
		}
		return worksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "following_users":
		var args struct {
			ID    int64 `json:"id"`
			Limit int   `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID < 0 {
			return callResult{}, errors.New("id must not be negative")
		}
		users, err := b.FollowingUsers(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return usersResult(limitItems(users, clampListLimit(args.Limit))), nil
	case "followers":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		users, err := b.Followers(ctx)
		if err != nil {
			return failure(err), nil
		}
		return usersResult(limitItems(users, clampListLimit(args.Limit))), nil
	case "my_pixiv_users":
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
		users, err := b.MyPixivUsers(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return usersResult(limitItems(users, clampListLimit(args.Limit))), nil
	case "related_users":
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
		users, err := b.RelatedUsers(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return usersResult(limitItems(users, clampListLimit(args.Limit))), nil
	case "series_watchlist":
		var args struct {
			Kind  string `json:"kind"`
			Limit int    `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		kind, err := seriesKind(args.Kind)
		if err != nil {
			return callResult{}, err
		}
		series, err := b.SeriesWatchlist(ctx, kind)
		if err != nil {
			return failure(err), nil
		}
		return seriesResult(limitItems(series, clampListLimit(args.Limit)), kind), nil
	case "trending_tags":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		tags, err := b.TrendingTags(ctx)
		if err != nil {
			return failure(err), nil
		}
		return tagsResult(limitItems(tags, clampListLimit(args.Limit))), nil
	case "settings_summary":
		summary, err := b.SettingsSummary(ctx)
		if err != nil {
			return failure(err), nil
		}
		return textResult(summary), nil
	case "capabilities":
		var args struct{}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		return textResult(capabilitiesText()), nil
	case "help":
		var args struct {
			Tool string `json:"tool"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		return helpResult(strings.TrimSpace(args.Tool))
	case "download_macro":
		var args struct{}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		macro, err := b.DownloadMacro(ctx)
		if err != nil {
			return failure(err), nil
		}
		if strings.TrimSpace(macro) == "" {
			return textResult("no download macro set"), nil
		}
		return textResult(macro), nil
	case "analyze_download_macro":
		var args struct {
			Text string `json:"text"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if strings.TrimSpace(args.Text) == "" {
			return callResult{}, errors.New("text is required")
		}
		tmpl, err := download.Parse(args.Text)
		if err != nil {
			return failure(err), nil
		}
		// Render with a sample work so invalid format specifiers show up too.
		sample, err := tmpl.Render(download.Context{
			ID: 123456, Title: "Title", ArtistID: 1, ArtistName: "Artist",
			PublishTime: time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), Ext: "jpg",
		})
		if err != nil {
			return failure(err), nil
		}
		return textResult("download macro is valid\nsample: " + sample), nil
	case "history":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		entries, err := b.History(ctx)
		if err != nil {
			return failure(err), nil
		}
		return historyResult(limitItems(entries, clampListLimit(args.Limit))), nil
	case "extensions":
		var args struct{}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		return textResult("extensions: none\nPixeval Go has no extension host, so no extensions are loaded."), nil
	case "novel_content":
		var args struct {
			ID int64 `json:"id"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if args.ID <= 0 {
			return callResult{}, errors.New("id is required")
		}
		body, err := b.NovelContent(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return textResult(body), nil
	case "recommended_users":
		var args struct {
			Limit int `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		users, err := b.RecommendedUsers(ctx)
		if err != nil {
			return failure(err), nil
		}
		return usersResult(limitItems(users, clampListLimit(args.Limit))), nil
	case "comments":
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
		comments, err := b.Comments(ctx, args.ID)
		if err != nil {
			return failure(err), nil
		}
		return commentsResult(limitItems(comments, clampListLimit(args.Limit))), nil
	case "works":
		var args struct {
			IDs   []int64 `json:"ids"`
			Limit int     `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		ids := batchIDs(args.IDs)
		if len(ids) == 0 {
			return callResult{}, errors.New("ids is required")
		}
		works, err := b.Works(ctx, ids)
		if err != nil {
			return failure(err), nil
		}
		return worksResult(limitItems(works, clampListLimit(args.Limit))), nil
	case "users":
		var args struct {
			IDs   []int64 `json:"ids"`
			Limit int     `json:"limit"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		ids := batchIDs(args.IDs)
		if len(ids) == 0 {
			return callResult{}, errors.New("ids is required")
		}
		users, err := b.Users(ctx, ids)
		if err != nil {
			return failure(err), nil
		}
		return userDetailsResult(limitItems(users, clampListLimit(args.Limit))), nil
	case "download_tasks":
		var args struct{}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		tasks, err := b.DownloadTasks(ctx)
		if err != nil {
			return failure(err), nil
		}
		return downloadTasksResult(tasks), nil
	case "analyze_work_filter":
		var args struct {
			Text string `json:"text"`
		}
		if err := decodeArgs(raw, &args); err != nil {
			return callResult{}, err
		}
		if strings.TrimSpace(args.Text) == "" {
			return callResult{}, errors.New("text is required")
		}
		if _, err := filter.Parse(args.Text); err != nil {
			return failure(err), nil
		}
		return textResult("work filter is valid"), nil
	default:
		return callResult{}, fmt.Errorf("unknown tool: %q", name)
	}
}

// batchIDs keeps the positive ids, drops duplicates and caps the list at
// maxBatch, so one call loads no more than the schema promises.
func batchIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
		if len(out) == maxBatch {
			break
		}
	}
	return out
}

// workKind is the kind an illustration feed asks for: illust when unset.
func workKind(kind string) (string, error) {
	if kind == "" {
		return "illust", nil
	}
	if kind != "illust" && kind != "manga" {
		return "", errors.New(`kind must be "illust" or "manga"`)
	}
	return kind, nil
}

// seriesKind is the kind a watchlist asks for: manga when unset.
func seriesKind(kind string) (string, error) {
	if kind == "" {
		return "manga", nil
	}
	if kind != "manga" && kind != "novel" {
		return "", errors.New(`kind must be "manga" or "novel"`)
	}
	return kind, nil
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

func commentsResult(comments []pixiv.Comment) callResult {
	return listResult(comments, "no comments found", commentText)
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

func seriesResult(series []pixiv.SeriesSummary, kind string) callResult {
	return listResult(series, "no series found", func(s *pixiv.SeriesSummary) string {
		return seriesText(s, kind)
	})
}

func tagsResult(tags []pixiv.TrendingTag) callResult {
	return listResult(tags, "no trending tags found", tagText)
}

func historyResult(entries []HistoryWork) callResult {
	return listResult(entries, "no history recorded", historyText)
}

func userDetailsResult(users []pixiv.UserDetail) callResult {
	return listResult(users, "no users found", userDetailText)
}

func downloadTasksResult(tasks []DownloadTask) callResult {
	return listResult(tasks, "no download tasks", downloadTaskText)
}

// capabilitiesText describes the server and the tools it serves.
func capabilitiesText() string {
	names := make([]string, 0, len(toolDefs))
	for _, def := range toolDefs {
		names = append(names, def.Name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "server: pixeval-go 0.1.0\nprotocol: %s\nauth: bearer token on every request\n", protocolVersion)
	fmt.Fprintf(&b, "tools: %d\n%s\n", len(toolDefs), strings.Join(names, ", "))
	fmt.Fprintf(&b, "list limit: %d by default, %d at most", defaultListLimit, maxLimit)
	return b.String()
}

// helpResult lists every tool, or describes the one named.
func helpResult(name string) (callResult, error) {
	if name == "" {
		lines := make([]string, 0, len(toolDefs))
		for _, def := range toolDefs {
			lines = append(lines, def.Name+": "+def.Description)
		}
		return textResult(strings.Join(lines, "\n")), nil
	}
	for _, def := range toolDefs {
		if def.Name != name {
			continue
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s: %s\n", def.Name, def.Description)
		keys := make([]string, 0, len(def.InputSchema.Properties))
		for key := range def.InputSchema.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			p := def.InputSchema.Properties[key]
			fmt.Fprintf(&b, "- %s (%s): %s", key, p.Type, p.Description)
			if p.Default != nil {
				fmt.Fprintf(&b, " default %v", p.Default)
			}
			b.WriteString("\n")
		}
		if len(def.InputSchema.Required) > 0 {
			fmt.Fprintf(&b, "required: %s", strings.Join(def.InputSchema.Required, ", "))
		}
		return textResult(strings.TrimRight(b.String(), "\n")), nil
	}
	return callResult{}, fmt.Errorf("unknown tool: %q", name)
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

// commentText is one comment: its author, date and body.
func commentText(c *pixiv.Comment) string {
	if c == nil {
		return "no comment found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s\n", c.ID, c.User.Name)
	if !c.CreateDate.IsZero() {
		fmt.Fprintf(&b, "%s\n", c.CreateDate.Format("2006-01-02 15:04"))
	}
	b.WriteString(clip(c.Comment, 500))
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

// historyText is one work in the local history, with when it was opened.
func historyText(h *HistoryWork) string {
	if h == nil {
		return "no history entry found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s — %s\n", h.ID, h.Title, h.Artist)
	if !h.OpenedAt.IsZero() {
		fmt.Fprintf(&b, "opened %s\n", h.OpenedAt.Format(time.RFC3339))
	}
	fmt.Fprintf(&b, "https://www.pixiv.net/artworks/%d", h.ID)
	return b.String()
}

// downloadTaskText is one download task, with its state and progress.
func downloadTaskText(t *DownloadTask) string {
	if t == nil {
		return "no download task found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s #%d %s — %s\n", t.State, t.IllustID, t.Title, t.Artist)
	fmt.Fprintf(&b, "page %d", t.Page+1)
	if t.Total > 0 {
		fmt.Fprintf(&b, ", %d/%d bytes", t.Bytes, t.Total)
	}
	b.WriteString("\n")
	if t.Err != "" {
		fmt.Fprintf(&b, "error: %s\n", t.Err)
	}
	fmt.Fprintf(&b, "https://www.pixiv.net/artworks/%d", t.IllustID)
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

// seriesText is one series in the watchlist, with its page URL.
func seriesText(s *pixiv.SeriesSummary, kind string) string {
	if s == nil {
		return "no series found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s — %s\n", s.ID, s.Title, s.User.Name)
	fmt.Fprintf(&b, "%d work(s)\n", s.PublishedContentCount)
	if !s.LastPublishedContent.IsZero() {
		fmt.Fprintf(&b, "last published %s\n", s.LastPublishedContent.Format(time.RFC3339))
	}
	if kind == "novel" {
		fmt.Fprintf(&b, "https://www.pixiv.net/novel/series/%d", s.ID)
	} else {
		fmt.Fprintf(&b, "https://www.pixiv.net/user/%d/series/%d", s.User.ID, s.ID)
	}
	return b.String()
}

// tagText is one trending tag, with its translation when Pixiv gives one.
func tagText(t *pixiv.TrendingTag) string {
	if t == nil {
		return "no tag found"
	}
	if t.TranslatedName == "" {
		return t.Tag
	}
	return fmt.Sprintf("%s (%s)", t.Tag, t.TranslatedName)
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
