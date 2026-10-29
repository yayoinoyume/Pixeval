// Package saucenao searches SauceNAO for the source of an image URL.
package saucenao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// DefaultBaseURL is the public SauceNAO site.
const DefaultBaseURL = "https://saucenao.com"

var (
	// ErrNoResults is returned when SauceNAO found nothing for the image.
	ErrNoResults = errors.New("saucenao: no results")
	// ErrRateLimited is returned when SauceNAO refuses the request because the
	// key is out of searches.
	ErrRateLimited = errors.New("saucenao: rate limited")
)

// Entry is one matching source SauceNAO returned.
type Entry struct {
	Similarity float64
	Thumbnail  string
	Title      string
	Creator    string
	AuthorURL  string
	ExtURLs    []string
}

// Result is one search's answer.
type Result struct {
	Entries []Entry
}

// Client searches SauceNAO. BaseURL may point at a test server; empty uses
// DefaultBaseURL.
type Client struct {
	BaseURL string
}

// NewClient returns a client for the public SauceNAO site.
func NewClient() *Client {
	return &Client{BaseURL: DefaultBaseURL}
}

type searchResponse struct {
	Header struct {
		Status int `json:"status"`
	} `json:"header"`
	Results []struct {
		Header struct {
			Similarity string `json:"similarity"`
			Thumbnail  string `json:"thumbnail"`
		} `json:"header"`
		Data struct {
			Title     string    `json:"title"`
			Creator   textValue `json:"creator"`
			AuthorURL string    `json:"author_url"`
			ExtURLs   []string  `json:"ext_urls"`
		} `json:"data"`
	} `json:"results"`
}

// textValue reads a field SauceNAO sends as a string in some indexes and as a
// list of strings in others.
type textValue string

func (v *textValue) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = ""
		return nil
	}
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*v = textValue(text)
		return nil
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil {
		*v = textValue(strings.Join(list, ", "))
		return nil
	}
	return errors.New("saucenao: creator is neither a string nor a list")
}

func (c *Client) baseURL() string {
	if c == nil || c.BaseURL == "" {
		return DefaultBaseURL
	}
	return c.BaseURL
}

// Search asks SauceNAO for the sources of imageURL.
func (c *Client) Search(ctx context.Context, apiKey, imageURL string) (*Result, error) {
	q := url.Values{}
	q.Set("output_type", "2")
	q.Set("api_key", apiKey)
	q.Set("url", imageURL)
	endpoint := strings.TrimRight(c.baseURL(), "/") + "/search.php?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("saucenao: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("saucenao: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusTooManyRequests:
		return nil, ErrRateLimited
	default:
		return nil, fmt.Errorf("saucenao: HTTP %s", resp.Status)
	}

	var payload searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("saucenao: decode: %w", err)
	}
	if payload.Header.Status != 0 {
		return nil, fmt.Errorf("saucenao: status %d", payload.Header.Status)
	}
	if len(payload.Results) == 0 {
		return nil, ErrNoResults
	}
	entries := make([]Entry, len(payload.Results))
	for i, r := range payload.Results {
		similarity, _ := strconv.ParseFloat(r.Header.Similarity, 64)
		entries[i] = Entry{
			Similarity: similarity,
			Thumbnail:  r.Header.Thumbnail,
			Title:      r.Data.Title,
			Creator:    string(r.Data.Creator),
			AuthorURL:  r.Data.AuthorURL,
			ExtURLs:    r.Data.ExtURLs,
		}
	}
	return &Result{Entries: entries}, nil
}
