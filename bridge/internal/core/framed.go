package core

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Framed mode (undocumented; recorded from `hc attach`, see docs/protocol.md):
// after a `hello` line with framed:true and its reply line, every message is
//
//	channel u32 LE | kind u8 | length u32 LE | payload
//
// Channel 0 kind 0 carries JSON-RPC. On a stream channel, kind 0 is DATA
// (terminal bytes either way), 1 CREDIT (u32: bytes the viewer consumed),
// 2 CLOSE, 3 RESIZE (u16 cols, u16 rows).
const (
	KindData   byte = 0
	KindCredit byte = 1
	KindClose  byte = 2
	KindResize byte = 3
)

const maxFrame = 16 << 20

// Frame is one framed-mode message.
type Frame struct {
	Channel uint32
	Kind    byte
	Payload []byte
}

// AttachParams are terminal.attach's parameters.
type AttachParams struct {
	Terminal string `json:"terminal"`
	Cols     uint16 `json:"cols"`
	Rows     uint16 `json:"rows"`
	History  int    `json:"history,omitempty"`
	Observe  bool   `json:"observe,omitempty"`
}

// Attachment is a live raw stream to one terminal.
type Attachment struct {
	conn    io.ReadWriteCloser
	r       *bufio.Reader
	channel uint32
	wmu     sync.Mutex
	once    sync.Once
	// Terminal is the attach result's terminal object.
	Terminal json.RawMessage
}

// Attach opens a framed connection and attaches to a terminal. Output
// arrives through Next; the caller must Credit what it consumes.
func (c *Client) Attach(ctx context.Context, p AttachParams) (*Attachment, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	stop := closeOnDone(ctx, conn)
	fail := func(err error) (*Attachment, error) {
		stop()
		conn.Close()
		return nil, ctxErr(ctx, err)
	}
	if err := writeRequest(conn, "hello", map[string]any{"framed": true, "generation": 1}); err != nil {
		return fail(err)
	}
	r := bufio.NewReader(conn)
	if _, _, err := readResponse(r); err != nil {
		return fail(fmt.Errorf("hello: %w", err))
	}
	a := &Attachment{conn: conn, r: r}

	req, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": callID, "method": "terminal.attach", "params": p})
	if err := a.write(0, KindData, req); err != nil {
		return fail(err)
	}
	// The response arrives on channel 0; stream data may follow it.
	for {
		f, err := a.readFrame()
		if err != nil {
			return fail(err)
		}
		if f.Channel != 0 {
			continue
		}
		var m struct {
			ID     *int64    `json:"id"`
			Error  *RPCError `json:"error"`
			Result struct {
				Channel  uint32          `json:"channel"`
				Terminal json.RawMessage `json:"terminal"`
			} `json:"result"`
		}
		if err := json.Unmarshal(f.Payload, &m); err != nil {
			return fail(fmt.Errorf("attach response: %w", err))
		}
		if m.ID == nil || *m.ID != callID {
			continue
		}
		if m.Error != nil {
			return fail(m.Error)
		}
		a.channel = m.Result.Channel
		a.Terminal = m.Result.Terminal
		// From here the caller ends the stream with Close, which detaches
		// cleanly; ctx no longer closes the connection out from under it.
		stop()
		return a, nil
	}
}

func (a *Attachment) readFrame() (Frame, error) {
	var h [9]byte
	if _, err := io.ReadFull(a.r, h[:]); err != nil {
		return Frame{}, err
	}
	n := binary.LittleEndian.Uint32(h[5:])
	if n > maxFrame {
		return Frame{}, fmt.Errorf("framed: %d-byte frame is too large", n)
	}
	f := Frame{Channel: binary.LittleEndian.Uint32(h[:4]), Kind: h[4], Payload: make([]byte, n)}
	_, err := io.ReadFull(a.r, f.Payload)
	return f, err
}

func (a *Attachment) write(channel uint32, kind byte, payload []byte) error {
	buf := make([]byte, 9+len(payload))
	binary.LittleEndian.PutUint32(buf[:4], channel)
	buf[4] = kind
	binary.LittleEndian.PutUint32(buf[5:9], uint32(len(payload)))
	copy(buf[9:], payload)
	a.wmu.Lock()
	defer a.wmu.Unlock()
	_, err := a.conn.Write(buf)
	return err
}

// ErrDetached means the core closed the stream (the terminal ended or was lost).
var ErrDetached = errors.New("terminal stream closed")

// Next returns the next chunk of terminal output.
func (a *Attachment) Next() ([]byte, error) {
	for {
		f, err := a.readFrame()
		if err != nil {
			return nil, err
		}
		if f.Channel != a.channel {
			continue
		}
		switch f.Kind {
		case KindData:
			return f.Payload, nil
		case KindClose:
			return nil, ErrDetached
		}
	}
}

// Credit tells the core n more bytes were consumed, so it keeps sending.
func (a *Attachment) Credit(n int) error {
	var p [4]byte
	binary.LittleEndian.PutUint32(p[:], uint32(n))
	return a.write(a.channel, KindCredit, p[:])
}

// Input sends keystrokes / pasted bytes to the terminal.
func (a *Attachment) Input(b []byte) error { return a.write(a.channel, KindData, b) }

// Resize sets the viewer's size in cells.
func (a *Attachment) Resize(cols, rows uint16) error {
	var p [4]byte
	binary.LittleEndian.PutUint16(p[:2], cols)
	binary.LittleEndian.PutUint16(p[2:], rows)
	return a.write(a.channel, KindResize, p[:])
}

// Close detaches (the terminal keeps running) and closes the connection,
// unblocking Next. Safe to call more than once.
func (a *Attachment) Close() error {
	a.once.Do(func() {
		a.write(a.channel, KindClose, nil)
		a.conn.Close()
	})
	return nil
}
