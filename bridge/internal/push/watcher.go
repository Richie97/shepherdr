package push

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"herdr-ui/bridge/internal/core"
)

// Alert is an agent entering a state worth a notification.
type Alert struct {
	Address string
	State   string // blocked, done or error
	Kind    string // permission, question or auth (blocked only)
	Msg     string
	// Filled in from terminal.get when available.
	Name, App, Where, Title, Cwd string
}

// alertStates are the states that notify.
var alertStates = map[string]bool{"blocked": true, "done": true, "error": true}

// Watcher follows the core's event stream and calls Notify when an agent
// moves into an alert state.
type Watcher struct {
	Core   *core.Client
	Notify func(context.Context, Alert)
	Logger *slog.Logger
	// Cooldown suppresses repeats of the same terminal+state (flapping).
	Cooldown time.Duration

	mu     sync.Mutex
	states map[string]string // address -> last status state
	last   map[string]time.Time
	now    func() time.Time
}

type terminalState struct {
	Address string `json:"address"`
	Status  *struct {
		State string `json:"state"`
	} `json:"status"`
}

type notification struct {
	Method string `json:"method"`
	Params struct {
		Type     string          `json:"type"`
		Terminal string          `json:"terminal"`
		Data     json.RawMessage `json:"data"`
	} `json:"params"`
}

type statusData struct {
	Status *struct {
		State string `json:"state"`
		Kind  string `json:"kind"`
		Msg   string `json:"msg"`
	} `json:"status"`
}

// Run watches until ctx ends, resubscribing after errors.
func (w *Watcher) Run(ctx context.Context) {
	if w.Logger == nil {
		w.Logger = slog.Default()
	}
	if w.now == nil {
		w.now = time.Now
	}
	delay := time.Second
	for ctx.Err() == nil {
		err := w.watchOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		w.Logger.Warn("push watcher: stream ended, retrying", "err", err, "in", delay)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = min(delay*2, 30*time.Second)
	}
}

func (w *Watcher) watchOnce(ctx context.Context) error {
	s, err := w.Core.Subscribe(ctx, nil)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := w.loadSnapshot(s.First); err != nil {
		return err
	}
	w.Logger.Info("push watcher: subscribed")
	for {
		line, err := s.Next()
		if err != nil {
			return err
		}
		if bytes.Contains(line, []byte(`"method":"missed"`)) {
			return errors.New("fell behind the event stream; resubscribing")
		}
		if a, ok := w.handle(line); ok {
			w.Notify(ctx, w.enrich(ctx, a))
		}
	}
}

// loadSnapshot records current states without notifying: an agent that was
// already blocked when the bridge started is not news.
func (w *Watcher) loadSnapshot(first []byte) error {
	var resp struct {
		Result struct {
			Terminals []terminalState `json:"terminals"`
		} `json:"result"`
	}
	if err := json.Unmarshal(first, &resp); err != nil {
		return fmt.Errorf("decode snapshot: %w", err)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.states = map[string]string{}
	if w.last == nil {
		w.last = map[string]time.Time{}
	}
	for _, t := range resp.Result.Terminals {
		if t.Status != nil {
			w.states[t.Address] = t.Status.State
		}
	}
	return nil
}

// handle updates state from one stream line and reports whether it is an
// alert.
func (w *Watcher) handle(line []byte) (Alert, bool) {
	var n notification
	if err := json.Unmarshal(line, &n); err != nil {
		return Alert{}, false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	addr := n.Params.Terminal
	switch {
	case n.Method != "event":
		return Alert{}, false
	case n.Params.Type == "removed":
		delete(w.states, addr)
		return Alert{}, false
	case n.Params.Type == "created":
		var t terminalState
		if json.Unmarshal(n.Params.Data, &t) == nil && t.Status != nil {
			w.states[addr] = t.Status.State
		}
		return Alert{}, false
	case n.Params.Type != "status":
		return Alert{}, false
	}
	var d statusData
	if json.Unmarshal(n.Params.Data, &d) != nil || d.Status == nil {
		delete(w.states, addr)
		return Alert{}, false
	}
	prev := w.states[addr]
	cur := d.Status.State
	w.states[addr] = cur
	if cur == prev || !alertStates[cur] {
		return Alert{}, false
	}
	key := addr + "|" + cur
	if t, ok := w.last[key]; ok && w.now().Sub(t) < w.Cooldown {
		return Alert{}, false
	}
	w.last[key] = w.now()
	return Alert{Address: addr, State: cur, Kind: d.Status.Kind, Msg: d.Status.Msg}, true
}

// enrich adds the terminal's name, app and location for the message.
func (w *Watcher) enrich(ctx context.Context, a Alert) Alert {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := w.Core.Call(ctx, "terminal.get", map[string]string{"terminal": a.Address})
	if err != nil {
		var rpcErr *core.RPCError
		if !errors.As(err, &rpcErr) {
			w.Logger.Warn("push watcher: terminal.get failed", "terminal", a.Address, "err", err)
		}
		return a
	}
	var t struct {
		Name   *string           `json:"name"`
		Title  *string           `json:"title"`
		Cwd    string            `json:"cwd"`
		Labels map[string]string `json:"labels"`
		Status *struct {
			App string `json:"app"`
		} `json:"status"`
		Foreground *struct {
			Cwd string `json:"cwd"`
		} `json:"foreground"`
	}
	if json.Unmarshal(raw, &t) != nil {
		return a
	}
	if t.Name != nil {
		a.Name = *t.Name
	}
	if t.Title != nil {
		a.Title = *t.Title
	}
	if t.Status != nil {
		a.App = t.Status.App
	}
	a.Where = t.Labels["where"]
	a.Cwd = t.Cwd
	if t.Foreground != nil && t.Foreground.Cwd != "" {
		a.Cwd = t.Foreground.Cwd
	}
	return a
}

// Message turns an alert into notification text. hub names the hub's own
// machine for addresses without a machine prefix.
func Message(a Alert, hub string) Notification {
	who := a.Name
	if who == "" {
		who = a.App
	}
	if who == "" {
		who = a.Address
	}
	machine, _, found := strings.Cut(a.Address, "/")
	if !found {
		machine = hub
	}
	subtitle := machine
	if a.Where != "" {
		subtitle += " · " + a.Where
	}

	var title, body, category string
	switch a.State {
	case "blocked":
		category = "BLOCKED"
		title = who + " needs you"
		body = map[string]string{
			"permission": "Wants permission to continue",
			"question":   "Has a question",
			"auth":       "Needs you to sign in",
		}[a.Kind]
		if body == "" {
			body = "Waiting for you"
		}
	case "error":
		category = "ERROR"
		title = who + " hit an error"
	default:
		category = "DONE"
		title = who + " finished"
	}
	switch {
	case a.Msg != "":
		body = a.Msg
	case body == "" && a.Title != "":
		body = a.Title
	case body == "" && a.Cwd != "":
		body = a.Cwd
	}
	return Notification{
		Title: title, Subtitle: subtitle, Body: body,
		Category:      category,
		ThreadID:      a.Address,
		CollapseID:    a.Address,
		TimeSensitive: a.State == "blocked",
		Data:          map[string]string{"address": a.Address, "state": a.State, "app": a.App},
	}
}
