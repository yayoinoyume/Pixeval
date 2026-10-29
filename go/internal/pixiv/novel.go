package pixiv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// NovelContent fetches the body of a novel. The reader page is HTML with the
// payload embedded, so it is fetched undecoded and mined below.
func (c *Client) NovelContent(ctx context.Context, novelID int64) (string, error) {
	q := url.Values{}
	q.Set("id", strconv.FormatInt(novelID, 10))
	var page string
	if err := c.get(ctx, c.pageURL("/webview/v2/novel", q), &rawText{text: &page}); err != nil {
		return "", err
	}
	return novelText(page)
}

// AddNovelBookmark bookmarks a novel. A private bookmark is visible only to
// the user.
func (c *Client) AddNovelBookmark(ctx context.Context, novelID int64, private bool) error {
	form := url.Values{}
	form.Set("novel_id", strconv.FormatInt(novelID, 10))
	form.Set("restrict", restrictParam(private))
	return c.post(ctx, "/v2/novel/bookmark/add", form, nil)
}

// RemoveNovelBookmark removes the user's bookmark from a novel.
func (c *Client) RemoveNovelBookmark(ctx context.Context, novelID int64) error {
	form := url.Values{}
	form.Set("novel_id", strconv.FormatInt(novelID, 10))
	return c.post(ctx, "/v1/novel/bookmark/delete", form, nil)
}

// novelText returns the text field of the novel object embedded in a reader page.
func novelText(page string) (string, error) {
	obj, err := embeddedNovel(page)
	if err != nil {
		return "", err
	}
	var r struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(obj, &r); err != nil {
		return "", fmt.Errorf("pixiv: decode novel: %w", err)
	}
	return r.Text, nil
}

// embeddedNovel cuts the JSON object out of the page. Pixiv writes it as a
// novel:{...} literal, and the object itself is too deeply nested for a regex,
// so the closing brace is found by counting braces outside of strings.
func embeddedNovel(page string) ([]byte, error) {
	for _, marker := range []string{"novel:", `"novel":`} {
		_, rest, found := strings.Cut(page, marker)
		if !found {
			continue
		}
		if end := objectEnd(rest); end > 0 {
			return []byte(rest[:end]), nil
		}
	}
	return nil, errors.New("pixiv: novel page without a novel object")
}

// objectEnd returns the length of the JSON object that starts at s, or 0.
func objectEnd(s string) int {
	i := 0
	for i < len(s) && strings.ContainsRune(" \t\r\n", rune(s[i])) {
		i++
	}
	if i >= len(s) || s[i] != '{' {
		return 0
	}
	depth := 0
	inString, escaped := false, false
	for ; i < len(s); i++ {
		switch ch := s[i]; {
		case escaped:
			escaped = false
		case inString && ch == '\\':
			escaped = true
		case ch == '"':
			inString = !inString
		case inString:
		case ch == '{':
			depth++
		case ch == '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return 0
}
