package extensions

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// rpcRequest is one JSON-RPC 2.0 request, written as a single line.
type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcResponse is one JSON-RPC 2.0 response, read from a single line.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

// rpcError is the error object of a failed request.
type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return "extensions: rpc error"
	}
	return fmt.Sprintf("extensions: rpc error %d: %s", e.Code, e.Message)
}

// process is one running extension executable. Calls are serialized: the
// protocol is request/response over one pipe, so a second request cannot
// overtake the first.
type process struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	mu     sync.Mutex
	nextID int64
	// broken marks a process that timed out, exited or spoke something other
	// than JSON-RPC; it is never reused.
	broken   bool
	done     bool
	stopOnce sync.Once
}

// call sends one request and waits for its response. A timeout kills the
// process: a late answer must never be read as the next request's result.
func (p *process) call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.broken {
		return nil, errors.New("extensions: process is not running")
	}
	id := p.nextID
	p.nextID++
	data, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if _, err := p.stdin.Write(data); err != nil {
		p.broken = true
		return nil, fmt.Errorf("extensions: %s: %w", method, err)
	}

	type readResult struct {
		response rpcResponse
		err      error
	}
	lines := make(chan readResult, 1)
	go func() {
		for {
			line, err := p.stdout.ReadBytes('\n')
			if err != nil {
				lines <- readResult{err: err}
				return
			}
			var response rpcResponse
			if err := json.Unmarshal(line, &response); err != nil {
				lines <- readResult{err: fmt.Errorf("extensions: invalid response: %w", err)}
				return
			}
			if response.ID != id {
				continue
			}
			lines <- readResult{response: response}
			return
		}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-timer.C:
		p.broken = true
		killProcess(p.cmd.Process)
		return nil, fmt.Errorf("extensions: %s timed out after %s", method, timeout)
	case result := <-lines:
		if result.err != nil {
			p.broken = true
			return nil, result.err
		}
		if result.response.Error != nil {
			return nil, result.response.Error
		}
		if len(result.response.Result) == 0 {
			return nil, errors.New("extensions: empty response result")
		}
		return result.response.Result, nil
	}
}

// stop asks the extension to shut down, then waits for it to exit. timeout is
// the whole budget for the request and the wait. On Linux a kill stops the
// extension's process group, so a wrapper script's children do not outlive
// it; other systems kill only the direct child.
func (p *process) stop(timeout time.Duration) {
	p.stopOnce.Do(func() {
		deadline := time.Now().Add(timeout)
		p.mu.Lock()
		broken := p.broken
		p.mu.Unlock()
		if !broken {
			_, _ = p.call("shutdown", nil, time.Until(deadline))
		}
		p.mu.Lock()
		broken = p.broken
		p.mu.Unlock()
		if broken {
			killProcess(p.cmd.Process)
		}
		done := make(chan struct{})
		go func() {
			_ = p.cmd.Wait()
			close(done)
		}()
		remaining := time.Until(deadline)
		if remaining < 0 {
			remaining = 0
		}
		select {
		case <-done:
		case <-time.After(remaining):
			killProcess(p.cmd.Process)
			<-done
		}
		_ = p.stdin.Close()
		p.mu.Lock()
		p.done = true
		p.mu.Unlock()
	})
}

// exited reports whether stop has waited for the process.
func (p *process) exited() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.done
}
