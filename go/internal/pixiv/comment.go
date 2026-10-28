package pixiv

import (
	"context"
	"net/url"
	"strconv"
	"time"
)

// Comment is one comment on a work; a reply carries the comment it answers.
type Comment struct {
	ID            int64     `json:"id"`
	Comment       string    `json:"comment"`
	User          User      `json:"user"`
	CreateDate    time.Time `json:"date"`
	HasReplies    bool      `json:"has_replies"`
	ParentComment *Comment  `json:"parent_comment"`
}

type commentList struct {
	Comments []Comment `json:"comments"`
	NextURL  string    `json:"next_url"`
}

// Comments reads one page of comments on a work.
func (c *Client) Comments(ctx context.Context, illustID int64, offset int) (*Page[Comment], error) {
	q := url.Values{}
	q.Set("illust_id", strconv.FormatInt(illustID, 10))
	q.Set("offset", strconv.Itoa(offset))
	return c.commentPage(ctx, c.pageURL("/v3/illust/comments", q))
}

// NextComments loads the page a NextURL points to.
func (c *Client) NextComments(ctx context.Context, nextURL string) (*Page[Comment], error) {
	return c.commentPage(ctx, nextURL)
}

func (c *Client) commentPage(ctx context.Context, rawURL string) (*Page[Comment], error) {
	var r commentList
	if err := c.get(ctx, rawURL, &r); err != nil {
		return nil, err
	}
	return &Page[Comment]{Items: r.Comments, NextURL: r.NextURL}, nil
}

// AddComment posts a comment on a work and returns the stored comment.
func (c *Client) AddComment(ctx context.Context, illustID int64, text string) (*Comment, error) {
	form := url.Values{}
	form.Set("illust_id", strconv.FormatInt(illustID, 10))
	form.Set("comment", text)
	var r struct {
		Comment Comment `json:"comment"`
	}
	if err := c.post(ctx, "/v1/illust/comment/add", form, &r); err != nil {
		return nil, err
	}
	return &r.Comment, nil
}

// DeleteComment removes one of the user's own comments.
func (c *Client) DeleteComment(ctx context.Context, commentID int64) error {
	form := url.Values{}
	form.Set("comment_id", strconv.FormatInt(commentID, 10))
	return c.post(ctx, "/v1/illust/comment/delete", form, nil)
}
