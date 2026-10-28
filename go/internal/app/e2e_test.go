package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/yayoinoyume/pixeval-go/internal/download"
	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

const (
	// e2eWorkID is the work every fake endpoint answers with.
	e2eWorkID = 7001
	// e2eToken is the access token the test signs in with.
	e2eToken = "e2e-access-token"
	// e2eTitle is the work's title, which the browse page has to show.
	e2eTitle = "端到端作品"
	// e2eArtist is the work's artist, whose name names the download folder.
	e2eArtist = "端到端画师"
	// e2eTimeout bounds the whole flow, so a hung request fails the test
	// instead of the suite.
	e2eTimeout = 5 * time.Second
	// appAPIHost is the host pixiv.Client's private base URL names, which the
	// test transport sends to the fake server.
	appAPIHost = "app-api.pixiv.net"
)

// e2eRequest is one request the fake Pixiv received.
type e2eRequest struct {
	path    string
	auth    string
	referer string
}

// e2ePixiv is a test server standing in for the app API and the image CDN.
// Its base URL is only known once the server runs, so the handlers read it
// under the lock instead of closing over it.
type e2ePixiv struct {
	mu   sync.Mutex
	base string
	jpeg []byte
	reqs []e2eRequest
}

func (p *e2ePixiv) setBase(base string) {
	p.mu.Lock()
	p.base = base
	p.mu.Unlock()
}

// imageURL is where the fake CDN serves the work's only page.
func (p *e2ePixiv) imageURL() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return fmt.Sprintf("%s/img/%d_p0.jpg", p.base, e2eWorkID)
}

// work is the illustration every endpoint answers with.
func (p *e2ePixiv) work() pixiv.Illustration {
	img := p.imageURL()
	return pixiv.Illustration{
		ID:             e2eWorkID,
		Title:          e2eTitle,
		Type:           "illust",
		User:           pixiv.User{ID: 9001, Name: e2eArtist, Account: "e2e"},
		ImageUrls:      pixiv.ImageUrls{SquareMedium: img, Medium: img, Large: img, Original: img},
		MetaSinglePage: pixiv.MetaSinglePage{OriginalImageURL: img},
		PageCount:      1,
		Width:          40,
		Height:         30,
	}
}

func (p *e2ePixiv) record(r *http.Request) {
	p.mu.Lock()
	p.reqs = append(p.reqs, e2eRequest{
		path:    r.URL.Path,
		auth:    r.Header.Get("Authorization"),
		referer: r.Header.Get("Referer"),
	})
	p.mu.Unlock()
}

func (p *e2ePixiv) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handler serves the two app API endpoints the flow reads and the CDN image
// its download fetches.
func (p *e2ePixiv) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/illust/recommended", func(w http.ResponseWriter, r *http.Request) {
		p.record(r)
		p.writeJSON(w, struct {
			Illusts []pixiv.Illustration `json:"illusts"`
			NextURL string               `json:"next_url"`
		}{Illusts: []pixiv.Illustration{p.work()}})
	})
	mux.HandleFunc("/v1/illust/detail", func(w http.ResponseWriter, r *http.Request) {
		p.record(r)
		if got := r.URL.Query().Get("illust_id"); got != strconv.Itoa(e2eWorkID) {
			http.Error(w, "unexpected illust_id "+got, http.StatusNotFound)
			return
		}
		p.writeJSON(w, struct {
			Illust pixiv.Illustration `json:"illust"`
		}{p.work()})
	})
	mux.HandleFunc("/img/", func(w http.ResponseWriter, r *http.Request) {
		p.record(r)
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(p.jpeg)
	})
	return mux
}

// requests returns the recorded requests whose path starts with prefix.
func (p *e2ePixiv) requests(prefix string) []e2eRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []e2eRequest
	for _, r := range p.reqs {
		if strings.HasPrefix(r.path, prefix) {
			out = append(out, r)
		}
	}
	return out
}

// paths lists every request path, for a failure message.
func (p *e2ePixiv) paths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]string, len(p.reqs))
	for i, r := range p.reqs {
		out[i] = r.path
	}
	return out
}

// e2eJPEG is the encoded 40x30 JPEG the fake CDN serves.
func e2eJPEG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for y := range 30 {
		for x := range 40 {
			img.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 8), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("编码 JPEG: %v", err)
	}
	return buf.Bytes()
}

// e2eTransport sends the app API's requests to the fake server. pixiv.Client
// keeps its base URL private and has no setter, so the transport is the seam
// the other app tests use too: the URL, the headers and the JSON decoding stay
// in the real path, only the host changes.
type e2eTransport struct{ target *url.URL }

func (tr e2eTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host != appAPIHost {
		return http.DefaultTransport.RoundTrip(req)
	}
	r := req.Clone(req.Context())
	r.URL.Scheme, r.URL.Host = tr.target.Scheme, tr.target.Host
	r.Host = tr.target.Host
	return http.DefaultTransport.RoundTrip(r)
}

// waitE2EFeed waits for the feed's first page, failing on the error the page
// would show or on the deadline.
func waitE2EFeed(t *testing.T, st *browseState) {
	t.Helper()
	deadline := time.Now().Add(e2eTimeout)
	for {
		v := st.snapshot()
		switch {
		case v.err != "":
			t.Fatalf("首页加载失败: %q", v.err)
		case len(v.items) > 0:
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%v 内首页没有加载: started=%v loading=%v", e2eTimeout, v.started, v.loading)
		}
		time.Sleep(time.Millisecond)
	}
}

// TestE2E runs the signed-in path end to end against a fake Pixiv: the home
// feed over HTTP, a detail request, the viewer, and a download whose bytes must
// match what the fake CDN served. Nothing reaches the real network.
func TestE2E(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, "config"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(dir, "cache"))

	fake := &e2ePixiv{jpeg: e2eJPEG(t)}
	srv := httptest.NewServer(fake.handler())
	t.Cleanup(srv.Close)
	fake.setBase(srv.URL)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("解析测试服务器地址: %v", err)
	}

	a := New()
	a.client.HTTP().Transport = e2eTransport{target: target}
	a.client.SetCooldown(0)
	a.downloadsDir = t.TempDir()
	a.applySession(&pixiv.Token{AccessToken: e2eToken, User: pixiv.TokenUser{Name: "端到端用户"}})

	// a. The home feed loads from the fake API as the browse page draws it.
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Fill().Children(func() {
			a.browsePage(c, a.homeFeeds[0])
		})
	}, 1000, 700)
	waitE2EFeed(t, a.homeFeeds[0])
	tt.Frame()
	if !tt.HasText(e2eTitle) {
		t.Fatalf("首页没有显示标题 %q: %q", e2eTitle, tt.Texts())
	}
	work := a.homeFeeds[0].snapshot().items[0]

	ctx, cancel := context.WithTimeout(context.Background(), e2eTimeout)
	defer cancel()

	// The detail request the app makes when it needs a work's originals.
	detail, err := a.illustrationFn(ctx, work.ID)
	if err != nil {
		t.Fatalf("详情请求: %v", err)
	}
	if detail.ID != e2eWorkID || detail.Title != e2eTitle {
		t.Fatalf("详情 %d %q, 期望 %d %q", detail.ID, detail.Title, e2eWorkID, e2eTitle)
	}

	// b. The viewer opens on the feed's work.
	a.openViewer(&work, 0)
	if a.viewer == nil {
		t.Fatal("查看器没有打开")
	}
	if a.viewer.work.ID != e2eWorkID {
		t.Fatalf("查看器显示作品 %d, 期望 %d", a.viewer.work.ID, e2eWorkID)
	}

	// c. The queue downloads the viewer's work to disk.
	queued, ok := a.downloadWork(a.viewer.work)
	if !ok {
		t.Fatal("没有下载目录, 无法排队")
	}
	if queued != 1 {
		t.Fatalf("排队 %d 个任务, 期望 1", queued)
	}
	a.downloads.Start(ctx)
	a.downloads.Wait()
	tasks := a.downloads.Snapshot()
	if len(tasks) != 1 {
		t.Fatalf("队列里有 %d 个任务, 期望 1", len(tasks))
	}
	if got := tasks[0]; got.State != download.Done {
		t.Fatalf("任务状态 %v, 错误 %q", got.State, got.Err)
	}
	saved, err := os.ReadFile(tasks[0].Dest)
	if err != nil {
		t.Fatalf("读取落盘文件 %s: %v", tasks[0].Dest, err)
	}
	if !bytes.Equal(saved, fake.jpeg) {
		t.Fatalf("落盘 %d 字节, 服务器返回 %d 字节, 内容不一致", len(saved), len(fake.jpeg))
	}

	// d. The fake server saw the feed, the detail and the image, with the token
	// on the API calls.
	for _, path := range []string{"/v1/illust/recommended", "/v1/illust/detail"} {
		reqs := fake.requests(path)
		if len(reqs) == 0 {
			t.Fatalf("服务器没有收到 %s; 收到 %v", path, fake.paths())
		}
		for _, r := range reqs {
			if want := "Bearer " + e2eToken; r.auth != want {
				t.Errorf("%s 的 Authorization = %q, 期望 %q", path, r.auth, want)
			}
		}
	}
	imgs := fake.requests("/img/")
	if len(imgs) == 0 {
		t.Fatalf("服务器没有收到图片请求; 收到 %v", fake.paths())
	}
	for _, r := range imgs {
		if r.referer != "https://www.pixiv.net" {
			t.Errorf("%s 的 Referer = %q, 期望 pixiv.net", r.path, r.referer)
		}
	}
}
