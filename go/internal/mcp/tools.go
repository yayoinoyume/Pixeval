package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

const (
	// defaultLimit is how many works a list tool returns when the call does
	// not say. maxLimit caps a client that asks for more.
	defaultLimit = 10
	maxLimit     = 50
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

var limitProperty = property{
	Type:        "integer",
	Description: "How many works to return, 1 to 50.",
	Default:     defaultLimit,
}

var idProperty = property{
	Type:        "integer",
	Description: "The work's Pixiv illustration id.",
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
