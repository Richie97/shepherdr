package core_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"herdr-ui/bridge/internal/core"
)

// When FAKE_HC=1 the test binary acts as `hc connect`: it answers each
// request on stdin with a result echoing the method, and for
// events.subscribe also sends one heartbeat.
func TestMain(m *testing.M) {
	if os.Getenv("FAKE_HC") == "1" {
		fakeHC()
		return
	}
	os.Exit(m.Run())
}

func fakeHC() {
	if len(os.Args) < 2 || os.Args[len(os.Args)-1] != "connect" {
		fmt.Fprintln(os.Stderr, "fake hc: want connect")
		os.Exit(2)
	}
	r := bufio.NewReader(os.Stdin)
	for {
		line, err := r.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
		}
		json.Unmarshal(line, &req)
		fmt.Printf(`{"jsonrpc":"2.0","id":%s,"result":{"method":%q}}`+"\n", req.ID, req.Method)
		if req.Method == "events.subscribe" {
			fmt.Println(`{"jsonrpc":"2.0","method":"heartbeat","params":{"seq":1}}`)
		}
	}
}

func fakeHCClient(t *testing.T) *core.Client {
	t.Setenv("FAKE_HC", "1")
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	// os.Args for the child will be [exe, "connect"]; flags parsing by the
	// test framework is skipped because TestMain returns first.
	return &core.Client{HCPath: exe}
}

func TestHCTransportCall(t *testing.T) {
	c := fakeHCClient(t)
	got, err := c.Call(context.Background(), "server.status", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), `"server.status"`) {
		t.Fatalf("result = %s", got)
	}
}

func TestHCTransportSubscribe(t *testing.T) {
	c := fakeHCClient(t)
	s, err := c.Subscribe(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	line, err := s.Next()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(line), "heartbeat") {
		t.Fatalf("line = %s", line)
	}
}
