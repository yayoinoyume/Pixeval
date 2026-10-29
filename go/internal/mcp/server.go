// Package mcp serves Pixeval Go to AI clients over the Model Context
// Protocol. The transport is Streamable HTTP in its stateless form: one
// JSON-RPC message per POST to /mcp, no session id and no server-sent events.
package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/yayoinoyume/pixeval-go/internal/pixiv"
)

// protocolVersion is the MCP revision this server speaks.
const protocolVersion = "2025-06-18"

// maxBody caps a request body: a call carries only arguments.
const maxBody = 1 << 20

// JSON-RPC 2.0 error codes the server sends.
const (
	codeParseError     = -32700
	codeInvalidRequest = -32600
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Backend is what the tools call. App implements it; tests use a fake.
type Backend interface {
	Search(ctx context.Context, query string, limit int) ([]pixiv.Illustration, error)
	Recommended(ctx context.Context, limit int) ([]pixiv.Illustration, error)
	Detail(ctx context.Context, id int64) (*pixiv.Illustration, error)
	Ranking(ctx context.Context, mode string, limit int) ([]pixiv.Illustration, error)
	SetBookmark(ctx context.Context, id int64, on bool) error
	QueueDownload(ctx context.Context, id int64) (int, error)
	Status() string
	SearchUsers(ctx context.Context, word string) ([]pixiv.UserPreview, error)
	UserDetail(ctx context.Context, id int64) (*pixiv.UserDetail, error)
	UserIllusts(ctx context.Context, id int64, kind string) ([]pixiv.Illustration, error)
	UserBookmarks(ctx context.Context, userID int64, private bool) ([]pixiv.Illustration, error)
	SearchNovels(ctx context.Context, word string) ([]pixiv.Novel, error)
	WatchLater(ctx context.Context) ([]SavedWork, error)
	DownloadHistory(ctx context.Context) ([]DownloadedWork, error)
	Subscriptions(ctx context.Context) ([]SubscriptionInfo, error)
	RelatedIllusts(ctx context.Context, id int64) ([]pixiv.Illustration, error)
	Spotlight(ctx context.Context) ([]pixiv.SpotlightArticle, error)
}

// Server answers MCP requests for one Backend. It keeps no session state, so a
// single value serves every client. Every request must carry the bearer token
// it was made with.
type Server struct {
	backend Backend
	token   string
}

// New is a Server for b that only accepts token as its bearer token. An empty
// token accepts nothing, so a misconfigured server stays closed.
func New(b Backend, token string) *Server {
	return &Server{backend: b, token: token}
}

// ListenAndServe serves b on addr at /mcp until the listener fails. Clients
// must send token in an Authorization: Bearer header.
func ListenAndServe(addr string, b Backend, token string) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", New(b, token))
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return srv.ListenAndServe()
}

// request is one JSON-RPC request or notification. A message without an id is
// a notification and gets no reply.
type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// content is one block of a tool result.
type content struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type callResult struct {
	Content []content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ServerInfo      serverInfo     `json:"serverInfo"`
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(r) {
		// The reply names no token: a client that guessed wrong only learns
		// that it must send the right one.
		w.Header().Set("WWW-Authenticate", `Bearer realm="pixeval-go"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var req request
	if err := json.Unmarshal(body, &req); err != nil {
		writeJSON(w, response{
			JSONRPC: "2.0",
			Error:   &rpcError{Code: codeParseError, Message: "parse error: " + err.Error()},
		})
		return
	}
	if req.Method == "" {
		writeJSON(w, response{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &rpcError{Code: codeInvalidRequest, Message: "request has no method"},
		})
		return
	}
	resp, ok := s.handle(r.Context(), req)
	if !ok {
		// A notification is acknowledged without a body.
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, resp)
}

// authorized reports whether r carries the bearer token the server was made
// with. Both sides are hashed first, so the comparison takes the same time
// whatever the guess is and does not depend on the token's length.
func (s *Server) authorized(r *http.Request) bool {
	if s.token == "" {
		return false
	}
	fields := strings.Fields(r.Header.Get("Authorization"))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bearer") {
		return false
	}
	want := sha256.Sum256([]byte(s.token))
	got := sha256.Sum256([]byte(fields[1]))
	return subtle.ConstantTimeCompare(got[:], want[:]) == 1
}

// handle answers req, reporting false when no reply is due.
func (s *Server) handle(ctx context.Context, req request) (response, bool) {
	resp := response{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = initializeResult{
			ProtocolVersion: protocolVersion,
			Capabilities:    map[string]any{"tools": map[string]any{}},
			ServerInfo:      serverInfo{Name: "pixeval-go", Version: "0.1.0"},
		}
	case "ping":
		resp.Result = struct{}{}
	case "tools/list":
		resp.Result = map[string]any{"tools": toolDefs}
	case "tools/call":
		result, err := s.callTool(ctx, req.Params)
		if err != nil {
			resp.Error = &rpcError{Code: codeInvalidParams, Message: err.Error()}
		} else {
			resp.Result = result
		}
	case "notifications/initialized", "notifications/cancelled":
		return response{}, false
	default:
		resp.Error = &rpcError{Code: codeMethodNotFound, Message: "method not found: " + req.Method}
	}
	if len(req.ID) == 0 {
		return response{}, false
	}
	return resp, true
}

// callTool runs the tool a tools/call names. An error it returns is a protocol
// error; a failure inside a tool is a result with isError set.
func (s *Server) callTool(ctx context.Context, raw json.RawMessage) (callResult, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if len(raw) == 0 || string(raw) == "null" {
		return callResult{}, errors.New("missing params")
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return callResult{}, fmt.Errorf("bad params: %w", err)
	}
	return runTool(ctx, s.backend, call.Name, call.Arguments)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("mcp: write: %v", err)
	}
}
