// Package core talks to a herdr-core server over its Unix socket.
//
// The protocol is JSON-RPC 2.0 with one message per line. Each call here
// opens its own connection: the socket is local and cheap, and a dedicated
// connection keeps long-running calls (terminal.wait, events.subscribe) from
// blocking anything else.
package core

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
)

// DefaultSocketPath returns the socket of the hc session named by HC_SESSION,
// or of the "default" session.
func DefaultSocketPath() string {
	session := os.Getenv("HC_SESSION")
	if session == "" {
		session = "default"
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "herdr-core", session, session+".sock")
}

// Client reaches a herdr-core server, either over its Unix socket or by
// running `hc connect`, which speaks the same protocol over stdin/stdout and
// works on every OS (on Windows the core is not on a Unix socket).
type Client struct {
	// SocketPath is the core's Unix socket. Used when HCPath is empty.
	SocketPath string
	// HCPath, when set, is the hc / herdr-core binary to run as `connect`.
	HCPath string
}

// RPCError is a JSON-RPC error returned by the core. Data usually carries a
// stable "kind" such as "not_found".
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return fmt.Sprintf("herdr-core: %s (code %d)", e.Message, e.Code) }

// message is any line on the socket: a response (ID set) or a notification
// (Method set).
type message struct {
	ID     *int64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

const callID = 1

func (c *Client) dial(ctx context.Context) (io.ReadWriteCloser, error) {
	if c.HCPath != "" {
		return dialHC(c.HCPath)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "unix", c.SocketPath)
	if err != nil {
		return nil, fmt.Errorf("connect to herdr-core at %s: %w", c.SocketPath, err)
	}
	return conn, nil
}

func writeRequest(conn io.Writer, method string, params any) error {
	req := struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int64  `json:"id"`
		Method  string `json:"method"`
		Params  any    `json:"params,omitempty"`
	}{"2.0", callID, method, params}
	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	_, err = conn.Write(append(b, '\n'))
	return err
}

// readResponse reads lines until the response to callID, skipping any
// notifications that arrive first. It returns the raw line too.
func readResponse(r *bufio.Reader) (json.RawMessage, []byte, error) {
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return nil, nil, err
		}
		var m message
		if err := json.Unmarshal(line, &m); err != nil {
			return nil, nil, fmt.Errorf("herdr-core sent invalid JSON: %w", err)
		}
		if m.ID == nil || *m.ID != callID {
			continue
		}
		if m.Error != nil {
			return nil, line, m.Error
		}
		return m.Result, line, nil
	}
}

// closeOnDone closes conn when ctx ends, unblocking any read or write.
// The returned func stops the watcher.
func closeOnDone(ctx context.Context, conn io.Closer) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

// Call performs one request and returns its result. A core-side failure is
// returned as *RPCError.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stop := closeOnDone(ctx, conn)
	defer stop()

	if err := writeRequest(conn, method, params); err != nil {
		return nil, ctxErr(ctx, err)
	}
	result, _, err := readResponse(bufio.NewReader(conn))
	return result, ctxErr(ctx, err)
}

func ctxErr(ctx context.Context, err error) error {
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Stream is an open events.subscribe connection.
type Stream struct {
	conn io.ReadWriteCloser
	r    *bufio.Reader
	stop func()
	// First is the raw subscribe response line: {"id":1,"result":{seq,terminals}}.
	First []byte
}

// Subscribe opens an events.subscribe stream. params may be nil or carry
// terminal, children_of and labels filters.
func (c *Client) Subscribe(ctx context.Context, params any) (*Stream, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	stop := closeOnDone(ctx, conn)
	if params == nil {
		params = struct{}{}
	}
	if err := writeRequest(conn, "events.subscribe", params); err != nil {
		stop()
		conn.Close()
		return nil, ctxErr(ctx, err)
	}
	r := bufio.NewReader(conn)
	_, first, err := readResponse(r)
	if err != nil {
		stop()
		conn.Close()
		return nil, ctxErr(ctx, err)
	}
	return &Stream{conn: conn, r: r, stop: stop, First: first}, nil
}

// Next returns the next raw line: an "event", "heartbeat" or "missed"
// notification. It returns io.EOF when the core closes the stream.
func (s *Stream) Next() ([]byte, error) {
	line, err := s.r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	return line, nil
}

// Close ends the stream.
func (s *Stream) Close() error {
	s.stop()
	return s.conn.Close()
}
