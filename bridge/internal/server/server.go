// Package server is the bridge's HTTP and WebSocket API (see docs/PLAN.md).
package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"

	"herdr-ui/bridge/internal/core"
	"herdr-ui/bridge/internal/pair"
	"herdr-ui/bridge/internal/push"
)

// Config configures the API.
type Config struct {
	Core    *core.Client
	Version string
	// Owner is the Tailscale login allowed to use the bridge. Requests must
	// carry it in Tailscale-User-Login, which `tailscale serve` sets.
	Owner string
	// NoAuth skips the identity check. Local development only.
	NoAuth bool
	Logger *slog.Logger
	// Push, when set, enables /v1/push/*.
	Push *push.Pusher
}

// maxRPCBody bounds /v1/rpc bodies; terminal.put carries file data.
const maxRPCBody = 16 << 20

// New returns the bridge's HTTP handler.
func New(cfg Config) http.Handler {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	s := &server{cfg: cfg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", s.health)
	mux.HandleFunc("POST /v1/rpc", s.rpc)
	mux.HandleFunc("GET /v1/ws", s.ws)
	mux.HandleFunc("GET /v1/attach", s.attach)
	mux.HandleFunc("POST /v1/start", s.start)
	mux.HandleFunc("GET /pair", s.pairPage)
	mux.HandleFunc("POST /v1/push/register", s.pushRegister)
	mux.HandleFunc("POST /v1/push/unregister", s.pushUnregister)
	mux.HandleFunc("POST /v1/push/test", s.pushTest)
	return s.auth(mux)
}

type server struct{ cfg Config }

// auth fails closed: with no owner configured, nothing is allowed.
func (s *server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.NoAuth {
			login := r.Header.Get("Tailscale-User-Login")
			if s.cfg.Owner == "" || !strings.EqualFold(login, s.cfg.Owner) {
				s.cfg.Logger.Warn("rejected request", "path", r.URL.Path, "login", login)
				writeError(w, http.StatusForbidden, "forbidden", "this bridge only serves its owner over tailscale serve")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *server) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	resp := map[string]any{
		"bridge_version": s.cfg.Version,
		"push_enabled":   s.cfg.Push != nil && s.cfg.Push.Enabled(),
	}
	hello, err := s.cfg.Core.Call(ctx, "hello", map[string]any{"generation": 1})
	if err != nil {
		resp["core_error"] = err.Error()
		writeJSON(w, http.StatusServiceUnavailable, resp)
		return
	}
	var h map[string]json.RawMessage
	if err := json.Unmarshal(hello, &h); err == nil {
		delete(h, "methods") // large and not useful to the phone
		resp["core"] = h
	}
	if machines, err := s.cfg.Core.Call(ctx, "machine.list", nil); err == nil {
		resp["machines"] = json.RawMessage(machines)
	}
	writeJSON(w, http.StatusOK, resp)
}

type rpcRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// rpc forwards one allowlisted call. Core errors come back with status 200
// and an "error" body mirroring JSON-RPC, so the app can read data.kind.
func (s *server) rpc(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRPCBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {\"method\": ..., \"params\": ...}")
		return
	}
	if !core.Allowed(req.Method) {
		writeError(w, http.StatusForbidden, "method_not_allowed", "the bridge does not forward "+req.Method)
		return
	}
	var params any
	if len(req.Params) > 0 && string(req.Params) != "null" {
		params = req.Params
	}
	audit := inputMethods[req.Method]
	if audit {
		s.cfg.Logger.Info("input", append([]any{"method", req.Method}, auditFields(req.Params)...)...)
	}
	result, err := s.cfg.Core.Call(r.Context(), req.Method, params)
	var rpcErr *core.RPCError
	switch {
	case errors.As(err, &rpcErr):
		if audit {
			s.cfg.Logger.Warn("input refused", "method", req.Method, "err", rpcErr.Message)
		}
		writeJSON(w, http.StatusOK, map[string]any{"error": rpcErr})
	case r.Context().Err() != nil:
		return // the client went away (e.g. left a screen); nothing to report
	case err != nil:
		s.cfg.Logger.Error("core call failed", "method", req.Method, "err", err)
		writeError(w, http.StatusBadGateway, "core_unavailable", err.Error())
	default:
		writeJSON(w, http.StatusOK, map[string]any{"result": result})
	}
}

// ws streams events.subscribe to the client: first the subscribe response
// line (snapshot), then every notification line, as WebSocket text messages.
// Query filters: terminal, children_of, label=k=v (repeatable).
func (s *server) ws(w http.ResponseWriter, r *http.Request) {
	params := map[string]any{}
	q := r.URL.Query()
	if v := q.Get("terminal"); v != "" {
		params["terminal"] = v
	}
	if v := q.Get("children_of"); v != "" {
		params["children_of"] = v
	}
	if labels := q["label"]; len(labels) > 0 {
		params["labels"] = labels
	}

	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return // Accept already wrote the response.
	}
	defer conn.CloseNow()
	// The client sends nothing; CloseRead handles control frames and cancels
	// ctx when the client goes away.
	ctx := conn.CloseRead(r.Context())

	stream, err := s.cfg.Core.Subscribe(ctx, params)
	if err != nil {
		s.cfg.Logger.Error("subscribe failed", "err", err)
		conn.Close(websocket.StatusInternalError, "herdr-core unavailable")
		return
	}
	defer stream.Close()

	line := stream.First
	for {
		wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := conn.Write(wctx, websocket.MessageText, trimNewline(line))
		cancel()
		if err != nil {
			return
		}
		if line, err = stream.Next(); err != nil {
			if ctx.Err() == nil {
				conn.Close(websocket.StatusTryAgainLater, "herdr-core stream ended")
			}
			return
		}
	}
}

// attach streams one terminal's raw output to the client and its input back:
// binary messages are terminal bytes both ways; text messages from the
// client are {"type":"resize","cols":N,"rows":N}. The first message to the
// client is {"type":"attached","terminal":{...}}.
// Query: terminal (required), cols, rows, history, observe=1.
func (s *server) attach(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	p := core.AttachParams{
		Terminal: q.Get("terminal"),
		Cols:     uint16(clampInt(q.Get("cols"), 80, 10, 500)),
		Rows:     uint16(clampInt(q.Get("rows"), 24, 4, 300)),
		History:  clampInt(q.Get("history"), 1000, 0, 10000),
		Observe:  q.Get("observe") == "1",
	}
	if p.Terminal == "" {
		writeError(w, http.StatusBadRequest, "bad_request", "terminal is required")
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20)

	// Hijacked connections don't cancel r.Context(); cancel when either side ends.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	att, err := s.cfg.Core.Attach(ctx, p)
	if err != nil {
		msg := err.Error()
		if len(msg) > 120 {
			msg = msg[:120]
		}
		conn.Close(websocket.StatusPolicyViolation, msg)
		return
	}
	defer att.Close()
	s.cfg.Logger.Info("attach", "terminal", p.Terminal, "observe", p.Observe, "cols", p.Cols, "rows", p.Rows)
	defer s.cfg.Logger.Info("detach", "terminal", p.Terminal)

	hello, _ := json.Marshal(map[string]any{"type": "attached", "terminal": att.Terminal})
	if err := conn.Write(ctx, websocket.MessageText, hello); err != nil {
		return
	}

	go func() {
		defer att.Close() // client gone: detach, which also ends att.Next below
		for {
			typ, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			if typ == websocket.MessageBinary {
				if !p.Observe {
					att.Input(data)
				}
				continue
			}
			var c struct {
				Type string `json:"type"`
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(data, &c) == nil && c.Type == "resize" && c.Cols > 0 && c.Rows > 0 && !p.Observe {
				att.Resize(c.Cols, c.Rows)
			}
		}
	}()

	for {
		b, err := att.Next()
		if err != nil {
			if errors.Is(err, core.ErrDetached) {
				conn.Close(websocket.StatusNormalClosure, "terminal stream closed")
			}
			return
		}
		wctx, wcancel := context.WithTimeout(ctx, 15*time.Second)
		err = conn.Write(wctx, websocket.MessageBinary, b)
		wcancel()
		if err != nil {
			return
		}
		// Credit only what reached the client, so a slow phone slows the stream.
		att.Credit(len(b))
	}
}

// pairPage shows a QR code that points the phone at this hub. The hub URL
// comes from the Host the browser used, which tailscale serve preserves.
func (s *server) pairPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := pair.Page(w, "https://"+r.Host); err != nil {
		s.cfg.Logger.Error("pair page", "err", err)
	}
}

func clampInt(s string, def, lo, hi int) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return max(lo, min(hi, n))
}

func trimNewline(b []byte) []byte {
	if n := len(b); n > 0 && b[n-1] == '\n' {
		return b[:n-1]
	}
	return b
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, kind, msg string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"kind": kind, "message": msg}})
}

type pushDevice struct {
	Token   string `json:"token"`
	Sandbox bool   `json:"sandbox"`
	Name    string `json:"name"`
}

func (s *server) pushDevices(w http.ResponseWriter) *push.Devices {
	if s.cfg.Push == nil || s.cfg.Push.Devices == nil {
		writeError(w, http.StatusNotImplemented, "push_unavailable", "push is not set up on this bridge")
		return nil
	}
	return s.cfg.Push.Devices
}

// pushRegister stores a device token. Registration works even before an
// APNs key is configured, so the phone doesn't need to retry later.
func (s *server) pushRegister(w http.ResponseWriter, r *http.Request) {
	devices := s.pushDevices(w)
	if devices == nil {
		return
	}
	var d pushDevice
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {\"token\", \"sandbox\", \"name\"}")
		return
	}
	if err := devices.Add(push.Device{Token: d.Token, Sandbox: d.Sandbox, Name: d.Name}); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registered": true, "push_enabled": s.cfg.Push.Enabled()})
}

func (s *server) pushUnregister(w http.ResponseWriter, r *http.Request) {
	devices := s.pushDevices(w)
	if devices == nil {
		return
	}
	var d pushDevice
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&d); err != nil {
		writeError(w, http.StatusBadRequest, "bad_request", "body must be {\"token\"}")
		return
	}
	if err := devices.Remove(d.Token); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"registered": false})
}

// pushTest sends a test notification to every device.
func (s *server) pushTest(w http.ResponseWriter, r *http.Request) {
	if s.pushDevices(w) == nil {
		return
	}
	sent, errs := s.cfg.Push.SendAll(r.Context(), push.Notification{
		Title: "Herdr", Subtitle: "Test", Body: "Push notifications are working.",
		Category: "TEST", ThreadID: "test",
	})
	msgs := make([]string, len(errs))
	for i, e := range errs {
		msgs[i] = e.Error()
	}
	writeJSON(w, http.StatusOK, map[string]any{"sent": sent, "errors": msgs})
}

// inputMethods change what a terminal sees; every one is logged so there is
// a record of what the phone typed where.
var inputMethods = map[string]bool{
	"terminal.send": true, "terminal.keys": true, "terminal.interrupt": true,
	"terminal.run": true, "terminal.put": true,
}

// auditFields summarizes params for the log: the target and keys, but only
// the length of typed text or uploaded data.
func auditFields(raw json.RawMessage) []any {
	var p struct {
		Terminal string   `json:"terminal"`
		Keys     []string `json:"keys"`
		Text     string   `json:"text"`
		Force    bool     `json:"force"`
		On       string   `json:"on"`
		Argv     []string `json:"argv"`
		Data     string   `json:"data"`
	}
	json.Unmarshal(raw, &p)
	f := []any{"terminal", p.Terminal}
	if p.Keys != nil {
		f = append(f, "keys", p.Keys)
	}
	if p.Text != "" {
		f = append(f, "text_len", len(p.Text))
	}
	if p.Force {
		f = append(f, "force", true)
	}
	if p.On != "" || p.Argv != nil {
		f = append(f, "on", p.On, "argv", p.Argv)
	}
	if p.Data != "" {
		f = append(f, "data_len", len(p.Data))
	}
	return f
}
