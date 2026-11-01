// Package machines keeps the hub's links to its other machines alive.
package machines

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"herdr-ui/bridge/internal/core"
)

// Healer redials machines the core has given up on. The core parks a
// machine in states like needs_login after an ssh hiccup (e.g. a Tailscale
// SSH re-check) and doesn't retry by itself; often a plain redial works once
// the cause has passed.
type Healer struct {
	Core     *core.Client
	Logger   *slog.Logger
	Interval time.Duration // how often to look (default 1 minute)
	MaxDelay time.Duration // longest backoff per machine (default 15 minutes)

	now     func() time.Time
	backoff map[string]time.Duration
	nextTry map[string]time.Time
}

// Run checks the machine list every Interval until ctx ends.
func (h *Healer) Run(ctx context.Context) {
	if h.Interval == 0 {
		h.Interval = time.Minute
	}
	t := time.NewTicker(h.Interval)
	defer t.Stop()
	for {
		h.Tick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// Tick runs one check: redial each machine that isn't connected and whose
// backoff has passed.
func (h *Healer) Tick(ctx context.Context) {
	if h.now == nil {
		h.now = time.Now
	}
	if h.MaxDelay == 0 {
		h.MaxDelay = 15 * time.Minute
	}
	if h.backoff == nil {
		h.backoff, h.nextTry = map[string]time.Duration{}, map[string]time.Time{}
	}
	lctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	raw, err := h.Core.Call(lctx, "machine.list", nil)
	cancel()
	if err != nil {
		return
	}
	var list struct {
		Machines []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"machines"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	for _, m := range list.Machines {
		if m.State == "connected" || m.State == "connecting" {
			delete(h.backoff, m.Name)
			delete(h.nextTry, m.Name)
			continue
		}
		if h.now().Before(h.nextTry[m.Name]) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		raw, err := h.Core.Call(cctx, "machine.connect", map[string]string{"machine": m.Name})
		cancel()
		var res struct {
			State string `json:"state"`
		}
		json.Unmarshal(raw, &res)
		if err == nil && res.State == "connected" {
			h.Logger.Info("machine reconnected", "machine", m.Name, "was", m.State)
			delete(h.backoff, m.Name)
			delete(h.nextTry, m.Name)
			continue
		}
		d := h.backoff[m.Name] * 2
		if d == 0 {
			d = h.Interval
		}
		d = min(d, h.MaxDelay)
		h.backoff[m.Name], h.nextTry[m.Name] = d, h.now().Add(d)
		h.Logger.Warn("machine still unreachable", "machine", m.Name, "state", m.State, "err", err, "retry_in", d)
	}
}
