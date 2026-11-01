package machines

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"herdr-ui/bridge/internal/core"
	"herdr-ui/bridge/internal/core/coretest"
)

func TestHealerRedialsWithBackoff(t *testing.T) {
	var mu sync.Mutex
	state := "needs_login"
	connectWorks := false
	fake := coretest.Start(t, func(req coretest.Request) [][]byte {
		mu.Lock()
		defer mu.Unlock()
		switch req.Method {
		case "machine.list":
			return [][]byte{coretest.Result(req, map[string]any{"machines": []map[string]string{
				{"name": "laptop", "state": state}, {"name": "box", "state": "connected"}}})}
		case "machine.connect":
			if connectWorks {
				state = "connected"
			}
			return [][]byte{coretest.Result(req, map[string]string{"name": "laptop", "state": state})}
		}
		return nil
	})
	now := time.Unix(0, 0)
	h := &Healer{Core: &core.Client{SocketPath: fake.Path}, Logger: slog.Default(), Interval: time.Minute,
		now: func() time.Time { return now }}
	connects := func() int {
		n := 0
		for _, r := range fake.Requests() {
			if r.Method == "machine.connect" {
				n++
				if string(r.Params) != `{"machine":"laptop"}` {
					t.Errorf("connect params = %s", r.Params)
				}
			}
		}
		return n
	}
	ctx := context.Background()
	h.Tick(ctx) // fails: retry in 1m
	h.Tick(ctx) // within backoff: skipped
	if n := connects(); n != 1 {
		t.Fatalf("connects = %d, want 1", n)
	}
	now = now.Add(61 * time.Second)
	h.Tick(ctx) // fails again: backoff 2m
	now = now.Add(90 * time.Second)
	h.Tick(ctx) // still within 2m
	if n := connects(); n != 2 {
		t.Fatalf("connects = %d, want 2", n)
	}
	mu.Lock()
	connectWorks = true
	mu.Unlock()
	now = now.Add(time.Minute)
	h.Tick(ctx) // succeeds
	h.Tick(ctx) // connected now: nothing to do
	if n := connects(); n != 3 || h.backoff["laptop"] != 0 {
		t.Fatalf("connects = %d backoff = %v", n, h.backoff)
	}
}
