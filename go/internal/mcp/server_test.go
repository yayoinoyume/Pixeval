package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// fakeBackend answers the tools without the network and records the calls.
type fakeBackend struct {
	works []pixiv.Illustration
	err   error

	searchQuery string
	searchLimit int
	limit       int
	mode        string
	detailID    int64
	bookmarkID  int64
	bookmarkOn  bool
	queuedID    int64
	queued      int
	status      string
}

func (f *fakeBackend) Search(_ context.Context, query string, limit int) ([]pixiv.Illustration, error) {
	f.searchQuery, f.searchLimit = query, limit
	return f.works, f.err
}

func (f *fakeBackend) Recommended(_ context.Context, limit int) ([]pixiv.Illustration, error) {
	f.limit = limit
	return f.works, f.err
}

func (f *fakeBackend) Detail(_ context.Context, id int64) (*pixiv.Illustration, error) {
	f.detailID = id
	if f.err != nil {
		return nil, f.err
	}
	if len(f.works) == 0 {
		return nil, errors.New("no work")
	}
	w := f.works[0]
	return &w, nil
}

func (f *fakeBackend) Ranking(_ context.Context, mode string, limit int) ([]pixiv.Illustration, error) {
	f.mode, f.limit = mode, limit
	return f.works, f.err
}

func (f *fakeBackend) SetBookmark(_ context.Context, id int64, on bool) error {
	f.bookmarkID, f.bookmarkOn = id, on
	return f.err
}

func (f *fakeBackend) QueueDownload(_ context.Context, id int64) (int, error) {
	f.queuedID = id
	return f.queued, f.err
}

func (f *fakeBackend) Status() string { return f.status }

// reply is a response with the fields a test checks left raw.
type reply struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// testToken is the bearer token the test servers require and the test client
// sends.
const testToken = "test-token"

// newTestServer is a server whose client knows testToken.
func newTestServer(b Backend) *Server {
	return New(b, testToken)
}

// call posts body to the server and decodes the reply.
func call(t *testing.T, h http.Handler, body string) reply {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	var r reply
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return r
}

// toolResult is the result of a tools/call.
type toolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func decodeResult(t *testing.T, r reply, out any) {
	t.Helper()
	if r.Error != nil {
		t.Fatalf("rpc error %d: %s", r.Error.Code, r.Error.Message)
	}
	if err := json.Unmarshal(r.Result, out); err != nil {
		t.Fatalf("decode result %s: %v", r.Result, err)
	}
}

func sampleWorks() []pixiv.Illustration {
	return []pixiv.Illustration{
		{
			ID:             101,
			Title:          "Rain",
			User:           pixiv.User{Name: "Ame"},
			Tags:           []pixiv.Tag{{Name: "original"}},
			TotalBookmarks: 42,
			Width:          800,
			Height:         1200,
			PageCount:      2,
		},
	}
}

func TestInitialize(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	r := call(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	var got struct {
		ProtocolVersion string         `json:"protocolVersion"`
		Capabilities    map[string]any `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	decodeResult(t, r, &got)
	if got.ProtocolVersion != "2025-06-18" {
		t.Errorf("protocolVersion %q", got.ProtocolVersion)
	}
	if _, ok := got.Capabilities["tools"]; !ok {
		t.Errorf("capabilities %v, want tools", got.Capabilities)
	}
	if got.ServerInfo.Name != "pixeval-go" || got.ServerInfo.Version != "0.1.0" {
		t.Errorf("serverInfo %+v", got.ServerInfo)
	}
}

func TestToolsList(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	r := call(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var got struct {
		Tools []struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			InputSchema json.RawMessage `json:"inputSchema"`
		} `json:"tools"`
	}
	decodeResult(t, r, &got)
	want := []string{
		"search_illustrations", "recommended", "illustration_detail", "ranking",
		"set_bookmark", "queue_download", "status",
	}
	if len(got.Tools) != len(want) {
		t.Fatalf("%d tools, want %d", len(got.Tools), len(want))
	}
	for i, name := range want {
		if got.Tools[i].Name != name {
			t.Errorf("tool %d is %q, want %q", i, got.Tools[i].Name, name)
		}
		if len(got.Tools[i].InputSchema) == 0 {
			t.Errorf("%s has no input schema", name)
		}
	}
}

func TestSearchCall(t *testing.T) {
	b := &fakeBackend{works: sampleWorks()}
	r := call(t, newTestServer(b), `{"jsonrpc":"2.0","id":3,"method":"tools/call",
		"params":{"name":"search_illustrations","arguments":{"query":"rain","limit":3}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if b.searchQuery != "rain" || b.searchLimit != 3 {
		t.Errorf("backend got query %q limit %d", b.searchQuery, b.searchLimit)
	}
	if len(got.Content) != 1 || got.Content[0].Type != "text" {
		t.Fatalf("content %+v", got.Content)
	}
	text := got.Content[0].Text
	for _, want := range []string{"#101", "Rain", "Ame", "https://www.pixiv.net/artworks/101"} {
		if !strings.Contains(text, want) {
			t.Errorf("text %q misses %q", text, want)
		}
	}
}

func TestSearchDefaultLimit(t *testing.T) {
	b := &fakeBackend{works: sampleWorks()}
	r := call(t, newTestServer(b), `{"jsonrpc":"2.0","id":4,"method":"tools/call",
		"params":{"name":"search_illustrations","arguments":{"query":"rain"}}}`)
	decodeResult(t, r, &toolResult{})
	if b.searchLimit != defaultLimit {
		t.Errorf("limit %d, want %d", b.searchLimit, defaultLimit)
	}
}

func TestToolErrorIsResult(t *testing.T) {
	b := &fakeBackend{err: errors.New("boom")}
	r := call(t, newTestServer(b), `{"jsonrpc":"2.0","id":5,"method":"tools/call",
		"params":{"name":"recommended","arguments":{}}}`)
	var got toolResult
	decodeResult(t, r, &got)
	if !got.IsError {
		t.Fatalf("isError false: %+v", got)
	}
	if len(got.Content) != 1 || !strings.Contains(got.Content[0].Text, "boom") {
		t.Errorf("content %+v", got.Content)
	}
}

func TestUnknownTool(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}), `{"jsonrpc":"2.0","id":6,"method":"tools/call",
		"params":{"name":"nope","arguments":{}}}`)
	if r.Error == nil || r.Error.Code != codeInvalidParams {
		t.Fatalf("reply %+v", r)
	}
}

func TestUnknownMethod(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}), `{"jsonrpc":"2.0","id":7,"method":"resources/list"}`)
	if r.Error == nil || r.Error.Code != codeMethodNotFound {
		t.Fatalf("reply %+v", r)
	}
}

func TestParseError(t *testing.T) {
	r := call(t, newTestServer(&fakeBackend{}), `{"jsonrpc":"2.0","id":8,`)
	if r.Error == nil || r.Error.Code != codeParseError {
		t.Fatalf("reply %+v", r)
	}
	if string(r.ID) != "null" {
		t.Errorf("id %q, want null", r.ID)
	}
}

func TestNotificationHasNoBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mcp",
		strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`))
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	newTestServer(&fakeBackend{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Errorf("status %d, want %d", rec.Code, http.StatusAccepted)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body %q, want empty", rec.Body.String())
	}
}

func TestGetIsMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer "+testToken)
	rec := httptest.NewRecorder()
	newTestServer(&fakeBackend{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d", rec.Code)
	}
}

// postAs posts body with header as the Authorization value, or none when it is
// empty, and returns the recorded reply.
func postAs(t *testing.T, h http.Handler, header, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(body))
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// TestRequestWithoutTokenIsUnauthorized checks a caller that sends no token
// gets 401 and nothing of the token back.
func TestRequestWithoutTokenIsUnauthorized(t *testing.T) {
	rec := postAs(t, newTestServer(&fakeBackend{}), "",
		`{"jsonrpc":"2.0","id":1,"method":"ping"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if body := rec.Body.String(); strings.Contains(body, testToken) {
		t.Errorf("body %q leaks the token", body)
	}
	if got := rec.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Bearer") {
		t.Errorf("WWW-Authenticate %q", got)
	}
}

// TestRequestWithWrongTokenIsUnauthorized checks a caller that guesses wrong
// is refused, whether the guess is another token or a bad header shape.
func TestRequestWithWrongTokenIsUnauthorized(t *testing.T) {
	h := newTestServer(&fakeBackend{})
	body := `{"jsonrpc":"2.0","id":1,"method":"ping"}`
	for _, header := range []string{
		"Bearer wrong-token",
		"Bearer ",
		testToken,
		"Basic " + testToken,
		"Bearer " + testToken + " extra",
	} {
		rec := postAs(t, h, header, body)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status %d, want %d", header, rec.Code, http.StatusUnauthorized)
		}
	}
}

// TestRequestWithTokenIsAnswered checks the right token, in any case of the
// scheme, reaches the tools.
func TestRequestWithTokenIsAnswered(t *testing.T) {
	rec := postAs(t, newTestServer(&fakeBackend{}), "bearer "+testToken,
		`{"jsonrpc":"2.0","id":2,"method":"ping"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d, body %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"result"`) {
		t.Errorf("body %q, want a result", rec.Body.String())
	}
}

// TestEmptyTokenRefusesEveryone checks a server made without a token stays
// closed, so a missing setting cannot expose the tools.
func TestEmptyTokenRefusesEveryone(t *testing.T) {
	h := New(&fakeBackend{}, "")
	for _, header := range []string{"", "Bearer ", "Bearer " + testToken} {
		rec := postAs(t, h, header, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("header %q: status %d, want %d", header, rec.Code, http.StatusUnauthorized)
		}
	}
}
