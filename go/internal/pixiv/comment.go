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
	CreateDate    time.Time `json:"create_date"`
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
	var r commentList
	if err := c.get(ctx, c.pageURL("/v3/illust/comments", q), &r); err != nil {
		return nil, err
	}
	return &Page[Comment]{Items: r.Comments, NextURL: r.NextURL}, nil
}

// AddComment posts a comment on a work.
func (c *Client) AddComment(ctx context.Context, illustID int64, text string) error {
	form := url.Values{}
	form.Set("illust_id", strconv.FormatInt(illustID, 10))
	form.Set("comment", text)
	return c.post(ctx, "/v1/illust/comment/add", form, nil)
}

// DeleteComment removes one of the user's own comments.
func (c *Client) DeleteComment(ctx context.Context, commentID int64) error {
	form := url.Values{}
	form.Set("comment_id", strconv.FormatInt(commentID, 10))
	return c.post(ctx, "/v1/illust/comment/delete", form, nil)
}
