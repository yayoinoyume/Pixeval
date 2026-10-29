package pixiv

import (
	"context"
	"testing"
)

// TestReadEndpointsFilterFollowsAllowR18 checks the user, series and illust
// detail reads drop the R18 filter only when the client allows R18 works.
func TestReadEndpointsFilterFollowsAllowR18(t *testing.T) {
	endpoints := map[string]func(*Client) error{
		"user detail":    func(c *Client) error { _, err := c.UserDetail(context.Background(), 42); return err },
		"user illusts":   func(c *Client) error { _, err := c.UserIllusts(context.Background(), 42, "manga"); return err },
		"user following": func(c *Client) error { _, err := c.UserFollowing(context.Background(), 42); return err },
		"user followers": func(c *Client) error { _, err := c.UserFollowers(context.Background(), 42); return err },
		"series works":   func(c *Client) error { _, err := c.SeriesWorks(context.Background(), 77); return err },
		"series detail":  func(c *Client) error { _, err := c.SeriesDetail(context.Background(), 77); return err },
		"illustration":   func(c *Client) error { _, err := c.Illustration(context.Background(), 7); return err },
	}
	for _, allow := range []bool{false, true} {
		for name, call := range endpoints {
			var got recordedRequest
			c := newWriteClient(t, recordInto(&got, `{}`))
			c.SetAllowR18(allow)
			if err := call(c); err != nil {
				t.Fatalf("%s (AllowR18=%v): %v", name, allow, err)
			}
			want := "for_android"
			if allow {
				want = ""
			}
			if q := got.query.Get("filter"); q != want {
				t.Errorf("%s (AllowR18=%v): filter = %q, want %q", name, allow, q, want)
			}
		}
	}
}
