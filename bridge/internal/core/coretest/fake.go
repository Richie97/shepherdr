// Package coretest provides a fake herdr-core socket server for tests.
package coretest

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Request is a request the fake received.
type Request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

// Handler answers one request. It returns the full raw response line(s) to
// write, without trailing newlines. For events.subscribe it may return the
// response followed by notification lines.
type Handler func(req Request) [][]byte

// Fake is a fake core listening on a Unix socket in a temp dir.
type Fake struct {
	Path string

	mu       sync.Mutex
	requests []Request
}

// Start starts a fake core. Connections stay open after the handler's lines
// are written until the client closes them, like a real subscription.
func Start(t *testing.T, h Handler) *Fake {
	t.Helper()
	// Unix socket paths are length-limited; t.TempDir() can be too long on macOS.
	dir, err := os.MkdirTemp("", "hc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &Fake{Path: filepath.Join(dir, "c.sock")}
	ln, err := net.Listen("unix", f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(conn, h)
		}
	}()
	return f
}

func (f *Fake) serve(conn net.Conn, h Handler) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			return
		}
		f.mu.Lock()
		f.requests = append(f.requests, req)
		f.mu.Unlock()
		for _, out := range h(req) {
			if _, err := conn.Write(append(out, '\n')); err != nil {
				return
			}
		}
	}
}

// Requests returns the requests received so far.
func (f *Fake) Requests() []Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Request(nil), f.requests...)
}

// Result builds a success response line for req.
func Result(req Request, result any) []byte {
	b, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	return b
}

// RawResult builds a success response line from an already-encoded result.
func RawResult(req Request, result json.RawMessage) []byte {
	return Result(req, result)
}

// Error builds an error response line for req.
func Error(req Request, code int, msg string, data any) []byte {
	b, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0", "id": req.ID,
		"error": map[string]any{"code": code, "message": msg, "data": data},
	})
	return b
}

// Fixture reads a file from bridge/testdata, given a path relative to it.
func Fixture(t *testing.T, root, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// FixtureResult returns the "result" field of a recorded response fixture.
func FixtureResult(t *testing.T, root, name string) json.RawMessage {
	t.Helper()
	var m struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(Fixture(t, root, name), &m); err != nil {
		t.Fatal(err)
	}
	return m.Result
}
