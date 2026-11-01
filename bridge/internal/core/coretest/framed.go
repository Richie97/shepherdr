package coretest

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
)

// FramedFake is a fake core that accepts one attach and records what the
// viewer sends on the stream.
type FramedFake struct {
	Path string
	// Output is sent as DATA right after the attach response.
	Output [][]byte
	// Got receives every frame the viewer sends on the stream channel.
	Got chan Frame
	// AttachParams receives the terminal.attach params.
	AttachParams chan json.RawMessage
	// CloseAfterOutput sends CLOSE after Output, like a terminal that ended.
	CloseAfterOutput bool
}

// Frame is a decoded frame.
type Frame struct {
	Channel uint32
	Kind    byte
	Payload []byte
}

// StartFramed starts a framed fake core.
func StartFramed(t *testing.T, output [][]byte) *FramedFake {
	t.Helper()
	dir, err := os.MkdirTemp("", "hcf")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	f := &FramedFake{Path: filepath.Join(dir, "c.sock"), Output: output,
		Got: make(chan Frame, 64), AttachParams: make(chan json.RawMessage, 4)}
	ln, err := net.Listen("unix", f.Path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func writeFrame(w io.Writer, ch uint32, kind byte, p []byte) error {
	h := make([]byte, 9, 9+len(p))
	binary.LittleEndian.PutUint32(h[:4], ch)
	h[4] = kind
	binary.LittleEndian.PutUint32(h[5:], uint32(len(p)))
	_, err := w.Write(append(h, p...))
	return err
}

func (f *FramedFake) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	if _, err := r.ReadBytes('\n'); err != nil { // hello
		return
	}
	c.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"framed":true}}` + "\n"))
	for {
		var h [9]byte
		if _, err := io.ReadFull(r, h[:]); err != nil {
			return
		}
		fr := Frame{Channel: binary.LittleEndian.Uint32(h[:4]), Kind: h[4], Payload: make([]byte, binary.LittleEndian.Uint32(h[5:]))}
		if _, err := io.ReadFull(r, fr.Payload); err != nil {
			return
		}
		if fr.Channel == 0 {
			var req struct {
				ID     json.RawMessage `json:"id"`
				Params json.RawMessage `json:"params"`
			}
			json.Unmarshal(fr.Payload, &req)
			f.AttachParams <- req.Params
			resp, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": req.ID,
				"result": map[string]any{"channel": 7, "terminal": map[string]any{"address": "t1"}}})
			writeFrame(c, 0, 0, resp)
			for _, o := range f.Output {
				writeFrame(c, 7, 0, o)
			}
			if f.CloseAfterOutput {
				writeFrame(c, 7, 2, nil)
			}
			continue
		}
		f.Got <- fr
	}
}
