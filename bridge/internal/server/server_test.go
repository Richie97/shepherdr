package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"herdr-ui/bridge/internal/core"
	"herdr-ui/bridge/internal/core/coretest"
	"herdr-ui/bridge/internal/push"
	"herdr-ui/bridge/internal/server"
)

const (
	testdata = "../../testdata"
	owner    = "me@example.com"
)

// fakeCore answers like the live core, from recorded fixtures.
func fakeCore(t *testing.T) *coretest.Fake {
	events := bytes.Split(bytes.TrimSpace(coretest.Fixture(t, testdata, "events.subscribe.jsonl")), []byte("\n"))
	return coretest.Start(t, func(req coretest.Request) [][]byte {
		switch req.Method {
		case "hello":
			return [][]byte{coretest.RawResult(req, coretest.FixtureResult(t, testdata, "hello.json"))}
		case "machine.list":
			return [][]byte{coretest.RawResult(req, coretest.FixtureResult(t, testdata, "machine.list.json"))}
		case "terminal.list":
			return [][]byte{coretest.RawResult(req, coretest.FixtureResult(t, testdata, "terminal.list.json"))}
		case "terminal.get":
			return [][]byte{coretest.Error(req, -32000, `no terminal "t9999"`, map[string]string{"kind": "not_found"})}
		case "events.subscribe":
			return events
		}
		return [][]byte{coretest.Error(req, -32601, "unexpected method in test", nil)}
	})
}

func newServer(t *testing.T, fake *coretest.Fake) *httptest.Server {
	h := server.New(server.Config{Core: &core.Client{SocketPath: fake.Path}, Version: "test", Owner: owner})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	return ts
}

func do(t *testing.T, ts *httptest.Server, method, path, login, body string) (int, map[string]any) {
	t.Helper()
	req, _ := http.NewRequest(method, ts.URL+path, strings.NewReader(body))
	if login != "" {
		req.Header.Set("Tailscale-User-Login", login)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("non-JSON body %q", b)
	}
	return resp.StatusCode, m
}

func TestAuthRejectsMissingAndWrongLogin(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	for _, login := range []string{"", "someone@else.com"} {
		if code, _ := do(t, ts, "GET", "/v1/health", login, ""); code != http.StatusForbidden {
			t.Errorf("login %q: status %d, want 403", login, code)
		}
	}
	if code, _ := do(t, ts, "GET", "/v1/health", "ME@example.com", ""); code != http.StatusOK {
		t.Errorf("owner (case-insensitive): status %d, want 200", code)
	}
}

func TestAuthFailsClosedWithoutOwner(t *testing.T) {
	fake := fakeCore(t)
	ts := httptest.NewServer(server.New(server.Config{Core: &core.Client{SocketPath: fake.Path}}))
	defer ts.Close()
	if code, _ := do(t, ts, "GET", "/v1/health", "anyone@x.com", ""); code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", code)
	}
}

func TestHealthDropsMethodsAndIncludesMachines(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	code, body := do(t, ts, "GET", "/v1/health", owner, "")
	if code != http.StatusOK {
		t.Fatalf("status %d: %v", code, body)
	}
	coreInfo := body["core"].(map[string]any)
	if _, ok := coreInfo["methods"]; ok {
		t.Error("health should not include the methods list")
	}
	if coreInfo["version"] == nil || body["machines"] == nil {
		t.Errorf("body = %v", body)
	}
}

func TestHealthReportsCoreDown(t *testing.T) {
	h := server.New(server.Config{Core: &core.Client{SocketPath: "/nonexistent/hc.sock"}, Owner: owner})
	ts := httptest.NewServer(h)
	defer ts.Close()
	code, body := do(t, ts, "GET", "/v1/health", owner, "")
	if code != http.StatusServiceUnavailable || body["core_error"] == nil {
		t.Fatalf("status %d body %v", code, body)
	}
}

func TestRPCForwardsAllowedMethod(t *testing.T) {
	fake := fakeCore(t)
	ts := newServer(t, fake)
	code, body := do(t, ts, "POST", "/v1/rpc", owner, `{"method":"terminal.list","params":{"all":false}}`)
	if code != http.StatusOK || body["result"] == nil {
		t.Fatalf("status %d body %v", code, body)
	}
	reqs := fake.Requests()
	if got := string(reqs[len(reqs)-1].Params); got != `{"all":false}` {
		t.Errorf("forwarded params = %s", got)
	}
}

func TestRPCRejectsDisallowedMethod(t *testing.T) {
	fake := fakeCore(t)
	ts := newServer(t, fake)
	code, _ := do(t, ts, "POST", "/v1/rpc", owner, `{"method":"server.stop"}`)
	if code != http.StatusForbidden {
		t.Fatalf("status %d, want 403", code)
	}
	if n := len(fake.Requests()); n != 0 {
		t.Fatalf("core got %d requests, want 0", n)
	}
}

func TestRPCPassesCoreErrorThrough(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	code, body := do(t, ts, "POST", "/v1/rpc", owner, `{"method":"terminal.get","params":{"terminal":"t9999"}}`)
	if code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	data := body["error"].(map[string]any)["data"].(map[string]any)
	if data["kind"] != "not_found" {
		t.Fatalf("error = %v", body["error"])
	}
}

func TestRPCBadBody(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	if code, _ := do(t, ts, "POST", "/v1/rpc", owner, `not json`); code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400", code)
	}
}

func TestWebSocketStreamsSnapshotThenEvents(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/v1/ws?label=where=x", &websocket.DialOptions{
		HTTPHeader: http.Header{"Tailscale-User-Login": {owner}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(-1)

	var kinds []string
	for range 9 {
		_, msg, err := conn.Read(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Result json.RawMessage `json:"result"`
			Method string          `json:"method"`
		}
		json.Unmarshal(msg, &m)
		if m.Result != nil {
			kinds = append(kinds, "snapshot")
		} else {
			kinds = append(kinds, m.Method)
		}
	}
	if kinds[0] != "snapshot" || kinds[1] != "event" || kinds[8] != "heartbeat" {
		t.Fatalf("messages = %v", kinds)
	}
}

func TestWebSocketRequiresAuth(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	_, resp, err := websocket.Dial(context.Background(), "ws"+strings.TrimPrefix(ts.URL, "http")+"/v1/ws", nil)
	if err == nil || resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("err %v resp %v, want 403", err, resp)
	}
}

func TestPushRegisterAndTest(t *testing.T) {
	fake := fakeCore(t)
	devices, err := push.OpenDevices(filepath.Join(t.TempDir(), "devices.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := &push.Pusher{Devices: devices, Hub: func() string { return "hub" }, Logger: slog.Default()}
	ts := httptest.NewServer(server.New(server.Config{Core: &core.Client{SocketPath: fake.Path}, Owner: owner, Push: p}))
	defer ts.Close()

	tok := strings.Repeat("0f", 32)
	code, body := do(t, ts, "POST", "/v1/push/register", owner, `{"token":"`+tok+`","sandbox":true,"name":"iPhone"}`)
	if code != http.StatusOK || body["registered"] != true || body["push_enabled"] != false {
		t.Fatalf("register: %d %v", code, body)
	}
	if l := devices.List(); len(l) != 1 || !l[0].Sandbox {
		t.Fatalf("devices = %+v", l)
	}
	if code, _ := do(t, ts, "POST", "/v1/push/register", owner, `{"token":"zz"}`); code != http.StatusBadRequest {
		t.Errorf("bad token: status %d", code)
	}
	// No APNs key configured: the test push reports why instead of failing silently.
	code, body = do(t, ts, "POST", "/v1/push/test", owner, ``)
	if code != http.StatusOK || body["sent"] != float64(0) || len(body["errors"].([]any)) != 1 {
		t.Fatalf("test: %d %v", code, body)
	}
	do(t, ts, "POST", "/v1/push/unregister", owner, `{"token":"`+tok+`"}`)
	if len(devices.List()) != 0 {
		t.Fatal("unregister did not remove the device")
	}
	if code, _ := do(t, ts, "POST", "/v1/push/register", "", `{}`); code != http.StatusForbidden {
		t.Errorf("unauthenticated register: status %d", code)
	}
}

func TestPushUnavailableWithoutPusher(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	if code, _ := do(t, ts, "POST", "/v1/push/register", owner, `{"token":"ab"}`); code != http.StatusNotImplemented {
		t.Fatalf("status %d, want 501", code)
	}
}

func TestRPCLogsInputWithoutText(t *testing.T) {
	fake := coretest.Start(t, func(req coretest.Request) [][]byte {
		return [][]byte{coretest.Result(req, map[string]any{"turn": 1})}
	})
	var buf bytes.Buffer
	h := server.New(server.Config{
		Core: &core.Client{SocketPath: fake.Path}, Owner: owner,
		Logger: slog.New(slog.NewTextHandler(&buf, nil)),
	})
	ts := httptest.NewServer(h)
	defer ts.Close()
	do(t, ts, "POST", "/v1/rpc", owner, `{"method":"terminal.keys","params":{"terminal":"t1","keys":["esc"]}}`)
	do(t, ts, "POST", "/v1/rpc", owner, `{"method":"terminal.send","params":{"terminal":"t1","text":"secret plan"}}`)
	do(t, ts, "POST", "/v1/rpc", owner, `{"method":"terminal.list"}`)
	log := buf.String()
	for _, want := range []string{"method=terminal.keys terminal=t1 keys=[esc]", "method=terminal.send terminal=t1 text_len=11"} {
		if !strings.Contains(log, want) {
			t.Errorf("log missing %q:\n%s", want, log)
		}
	}
	if strings.Contains(log, "secret plan") || strings.Contains(log, "terminal.list") {
		t.Errorf("log has text or a read call:\n%s", log)
	}
}

func TestAttachProxiesTerminal(t *testing.T) {
	fake := coretest.StartFramed(t, [][]byte{[]byte("prompt% ")})
	ts := httptest.NewServer(server.New(server.Config{Core: &core.Client{SocketPath: fake.Path}, Owner: owner}))
	defer ts.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(ts.URL, "http")+"/v1/attach?terminal=simrig/t9&cols=90&rows=33", &websocket.DialOptions{
		HTTPHeader: http.Header{"Tailscale-User-Login": {owner}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()

	var p map[string]any
	json.Unmarshal(<-fake.AttachParams, &p)
	if p["terminal"] != "simrig/t9" || p["cols"] != float64(90) || p["rows"] != float64(33) {
		t.Fatalf("attach params = %v", p)
	}
	typ, msg, _ := conn.Read(ctx)
	if typ != websocket.MessageText || !strings.Contains(string(msg), `"attached"`) {
		t.Fatalf("first message = %s", msg)
	}
	typ, msg, _ = conn.Read(ctx)
	if typ != websocket.MessageBinary || string(msg) != "prompt% " {
		t.Fatalf("output = %q", msg)
	}
	frame := func() coretest.Frame {
		select {
		case f := <-fake.Got:
			return f
		case <-ctx.Done():
			t.Fatal("no frame")
		}
		return coretest.Frame{}
	}
	if f := frame(); f.Kind != core.KindCredit {
		t.Fatalf("want credit after delivery, got %+v", f)
	}
	conn.Write(ctx, websocket.MessageBinary, []byte("ls\r"))
	if f := frame(); f.Kind != core.KindData || string(f.Payload) != "ls\r" {
		t.Fatalf("input = %+v", f)
	}
	conn.Write(ctx, websocket.MessageText, []byte(`{"type":"resize","cols":120,"rows":40}`))
	if f := frame(); f.Kind != core.KindResize {
		t.Fatalf("resize = %+v", f)
	}
	conn.Close(websocket.StatusNormalClosure, "")
	if f := frame(); f.Kind != core.KindClose {
		t.Fatalf("want detach on client close, got %+v", f)
	}
}

func TestStartAgentLikeHerdr(t *testing.T) {
	var mu sync.Mutex
	gets := 0
	fake := coretest.Start(t, func(req coretest.Request) [][]byte {
		switch req.Method {
		case "terminal.run":
			return [][]byte{coretest.Result(req, map[string]any{"address": "laptop/t5"})}
		case "terminal.send":
			return [][]byte{coretest.Result(req, map[string]any{"turn": 0})}
		case "terminal.get":
			mu.Lock()
			gets++
			state := "working"
			if gets >= 2 {
				state = "idle" // ready on the second poll
			}
			mu.Unlock()
			return [][]byte{coretest.Result(req, map[string]any{"state": "live", "status": map[string]string{"state": state}})}
		}
		return [][]byte{coretest.Error(req, -32601, "unexpected", nil)}
	})
	ts := newServer(t, fake)
	code, body := do(t, ts, "POST", "/v1/start", owner,
		`{"on":"laptop","cwd":"/src/app","command":"claude","task":"fix the tests","name":"fixer","space":"spdb3c431bc19042004ffe674965e62320"}`)
	if code != http.StatusOK || body["address"] != "laptop/t5" {
		t.Fatalf("start: %d %v", code, body)
	}
	if !strings.HasPrefix(body["tab"].(string), "tb") || len(body["tab"].(string)) != 34 {
		t.Errorf("new tab id = %v", body["tab"])
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var sends []string
		for _, r := range fake.Requests() {
			if r.Method == "terminal.send" {
				sends = append(sends, string(r.Params))
			}
		}
		if len(sends) == 2 {
			if !strings.Contains(sends[0], `"text":"claude"`) || !strings.Contains(sends[1], `"text":"fix the tests"`) {
				t.Fatalf("sends = %v", sends)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("task not delivered; sends = %v", sends)
		}
		time.Sleep(50 * time.Millisecond)
	}
	var run map[string]any
	json.Unmarshal(fake.Requests()[0].Params, &run)
	labels := run["labels"].(map[string]any)
	if run["on"] != "laptop" || run["cwd"] != "/src/app" || run["name"] != "fixer" || run["keep"] != true || run["no_parent"] != true ||
		labels["herdr.space"] != "spdb3c431bc19042004ffe674965e62320" || run["argv"] != nil {
		t.Fatalf("terminal.run params = %v", run)
	}
}

func TestStartValidates(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	for _, body := range []string{`{"name":"Bad Name"}`, `{"space":"../x"}`, `not json`} {
		if code, _ := do(t, ts, "POST", "/v1/start", owner, body); code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", body, code)
		}
	}
}

func TestPairPageRequiresOwnerAndShowsHub(t *testing.T) {
	ts := newServer(t, fakeCore(t))
	req, _ := http.NewRequest("GET", ts.URL+"/pair", nil)
	req.Header.Set("Tailscale-User-Login", owner)
	req.Host = "hub.example.ts.net:8443"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || !strings.Contains(string(b), "https://hub.example.ts.net:8443") || !strings.Contains(string(b), "<svg") {
		t.Fatalf("status %d body %.200s", resp.StatusCode, b)
	}
	if code, _ := doRaw(t, ts, "/pair"); code != http.StatusForbidden {
		t.Errorf("unauthenticated: %d", code)
	}
}

func doRaw(t *testing.T, ts *httptest.Server, path string) (int, string) {
	resp, err := http.Get(ts.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}
