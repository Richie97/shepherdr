package core_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"herdr-ui/bridge/internal/core"
	"herdr-ui/bridge/internal/core/coretest"
)

const testdata = "../../testdata"

func TestCallReturnsResultAndSkipsNotifications(t *testing.T) {
	list := coretest.FixtureResult(t, testdata, "terminal.list.json")
	fake := coretest.Start(t, func(req coretest.Request) [][]byte {
		return [][]byte{
			[]byte(`{"jsonrpc":"2.0","method":"heartbeat","params":{"seq":1}}`),
			coretest.RawResult(req, list),
		}
	})
	c := &core.Client{SocketPath: fake.Path}

	got, err := c.Call(context.Background(), "terminal.list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Terminals []struct {
			Address string `json:"address"`
		} `json:"terminals"`
	}
	if err := json.Unmarshal(got, &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Terminals) == 0 {
		t.Fatal("no terminals decoded from fixture")
	}
	if reqs := fake.Requests(); len(reqs) != 1 || reqs[0].Method != "terminal.list" {
		t.Fatalf("requests = %+v", reqs)
	}
}

func TestCallReturnsRPCError(t *testing.T) {
	errLine := coretest.Fixture(t, testdata, "error.unknown_terminal.json")
	fake := coretest.Start(t, func(req coretest.Request) [][]byte {
		var m map[string]json.RawMessage
		json.Unmarshal(errLine, &m)
		m["id"] = req.ID
		b, _ := json.Marshal(m)
		return [][]byte{b}
	})
	c := &core.Client{SocketPath: fake.Path}

	_, err := c.Call(context.Background(), "terminal.get", map[string]any{"terminal": "t9999"})
	var rpcErr *core.RPCError
	if !errors.As(err, &rpcErr) {
		t.Fatalf("err = %v, want *RPCError", err)
	}
	if !bytes.Contains(rpcErr.Data, []byte(`"not_found"`)) {
		t.Fatalf("data = %s", rpcErr.Data)
	}
}

func TestCallHonorsContext(t *testing.T) {
	fake := coretest.Start(t, func(req coretest.Request) [][]byte { return nil }) // never answers
	c := &core.Client{SocketPath: fake.Path}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := c.Call(ctx, "terminal.wait", nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want deadline exceeded", err)
	}
}

func TestSubscribeStreamsRecordedEvents(t *testing.T) {
	lines := bytes.Split(bytes.TrimSpace(coretest.Fixture(t, testdata, "events.subscribe.jsonl")), []byte("\n"))
	fake := coretest.Start(t, func(req coretest.Request) [][]byte { return lines })
	c := &core.Client{SocketPath: fake.Path}

	s, err := c.Subscribe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !bytes.Equal(s.First, append(lines[0], '\n')) {
		t.Fatalf("First = %s", s.First)
	}
	var types []string
	for range lines[1:] {
		line, err := s.Next()
		if err != nil {
			t.Fatal(err)
		}
		var n struct {
			Method string `json:"method"`
			Params struct {
				Type string `json:"type"`
			} `json:"params"`
		}
		json.Unmarshal(line, &n)
		types = append(types, n.Method+":"+n.Params.Type)
	}
	if types[0] != "event:created" || types[len(types)-1] != "heartbeat:" {
		t.Fatalf("types = %v", types)
	}
}

func TestAllowlist(t *testing.T) {
	for _, m := range []string{"terminal.list", "terminal.send", "terminal.keys"} {
		if !core.Allowed(m) {
			t.Errorf("%s should be allowed", m)
		}
	}
	for _, m := range []string{"server.stop", "terminal.kill", "machine.exec", "file.write", "doc.set", ""} {
		if core.Allowed(m) {
			t.Errorf("%s should not be allowed", m)
		}
	}
}
