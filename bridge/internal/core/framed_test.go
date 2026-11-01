package core_test

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"herdr-ui/bridge/internal/core"
	"herdr-ui/bridge/internal/core/coretest"
)

func next(t *testing.T, ch <-chan coretest.Frame) coretest.Frame {
	t.Helper()
	select {
	case f := <-ch:
		return f
	case <-time.After(2 * time.Second):
		t.Fatal("no frame from viewer")
		return coretest.Frame{}
	}
}

func TestAttachStreamsAndControls(t *testing.T) {
	fake := coretest.StartFramed(t, [][]byte{[]byte("hello\r\n")})
	c := &core.Client{SocketPath: fake.Path}
	a, err := c.Attach(context.Background(), core.AttachParams{Terminal: "simrig/t9", Cols: 80, Rows: 24, History: 100})
	if err != nil {
		t.Fatal(err)
	}
	var p map[string]any
	json.Unmarshal(<-fake.AttachParams, &p)
	if p["terminal"] != "simrig/t9" || p["cols"] != float64(80) || p["history"] != float64(100) {
		t.Fatalf("attach params = %v", p)
	}
	if string(a.Terminal) != `{"address":"t1"}` {
		t.Fatalf("terminal = %s", a.Terminal)
	}

	out, err := a.Next()
	if err != nil || string(out) != "hello\r\n" {
		t.Fatalf("Next = %q, %v", out, err)
	}
	a.Credit(len(out))
	if f := next(t, fake.Got); f.Channel != 7 || f.Kind != core.KindCredit || binary.LittleEndian.Uint32(f.Payload) != 7 {
		t.Fatalf("credit frame = %+v", f)
	}
	a.Input([]byte("ls\r"))
	if f := next(t, fake.Got); f.Kind != core.KindData || string(f.Payload) != "ls\r" {
		t.Fatalf("input frame = %+v", f)
	}
	a.Resize(100, 30)
	if f := next(t, fake.Got); f.Kind != core.KindResize || binary.LittleEndian.Uint16(f.Payload) != 100 || binary.LittleEndian.Uint16(f.Payload[2:]) != 30 {
		t.Fatalf("resize frame = %+v", f)
	}
	a.Close()
	if f := next(t, fake.Got); f.Kind != core.KindClose {
		t.Fatalf("close frame = %+v", f)
	}
}

func TestAttachReportsStreamClose(t *testing.T) {
	fake := coretest.StartFramed(t, [][]byte{[]byte("bye")})
	fake.CloseAfterOutput = true
	a, err := (&core.Client{SocketPath: fake.Path}).Attach(context.Background(), core.AttachParams{Terminal: "t1", Cols: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	a.Next()
	if _, err := a.Next(); !errors.Is(err, core.ErrDetached) {
		t.Fatalf("err = %v, want ErrDetached", err)
	}
}
