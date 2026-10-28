// Package mcp serves Pixeval Go to AI clients over the Model Context
// Protocol. The transport is Streamable HTTP in its stateless form: one
// JSON-RPC message per POST to /mcp, no session id and no server-sent events.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
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
}

// Server answers MCP requests for one Backend. It keeps no session state, so a
// single value serves every client.
type Server struct {
	backend Backend
}

// New is a Server for b.
func New(b Backend) *Server {
	return &Server{backend: b}
}

// ListenAndServe serves b on addr at /mcp until the listener fails.
func ListenAndServe(addr string, b Backend) error {
	mux := http.NewServeMux()
	mux.Handle("/mcp", New(b))
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
