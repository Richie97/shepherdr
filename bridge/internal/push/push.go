package push

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// Config is apns.json in the bridge's config dir. The key itself is
// apns.p8 next to it (never committed).
type Config struct {
	KeyID  string `json:"key_id"`
	TeamID string `json:"team_id"`
	Topic  string `json:"topic"`
}

// ConfigDir is where the bridge keeps apns.json, apns.p8 and devices.json:
// ~/Library/Application Support/herdr-bridge on macOS, %AppData%\herdr-bridge
// on Windows, ~/.config/herdr-bridge on Linux.
func ConfigDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "herdr-bridge"), nil
}

// LoadSender reads apns.json and apns.p8 from dir. It returns (nil, nil)
// when push isn't configured.
func LoadSender(dir string) (*Sender, error) {
	b, err := os.ReadFile(filepath.Join(dir, "apns.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("apns.json: %w", err)
	}
	p8, err := os.ReadFile(filepath.Join(dir, "apns.p8"))
	if err != nil {
		return nil, fmt.Errorf("apns.p8: %w", err)
	}
	return NewSender(p8, c.KeyID, c.TeamID, c.Topic)
}

// Pusher fans notifications out to every registered device.
type Pusher struct {
	Sender  *Sender // nil: push disabled, alerts are only logged
	Devices *Devices
	Hub     func() string
	Logger  *slog.Logger
}

// Enabled reports whether an APNs key is configured.
func (p *Pusher) Enabled() bool { return p.Sender != nil }

// Alert sends a notification for a to all devices.
func (p *Pusher) Alert(ctx context.Context, a Alert) {
	n := Message(a, p.Hub())
	p.Logger.Info("agent alert", "terminal", a.Address, "state", a.State, "title", n.Title)
	p.SendAll(ctx, n)
}

// SendAll sends n to all devices, forgetting ones APNs reports as gone.
// It returns how many deliveries succeeded.
func (p *Pusher) SendAll(ctx context.Context, n Notification) (sent int, errs []error) {
	if p.Sender == nil {
		return 0, []error{errors.New("push is not configured (no apns.json / apns.p8)")}
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, d := range p.Devices.List() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := p.Sender.Send(ctx, d.Token, d.Sandbox, n)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				sent++
			case errors.Is(err, ErrUnregistered):
				p.Logger.Info("forgetting device", "token", short(d.Token), "err", err)
				p.Devices.Remove(d.Token)
				errs = append(errs, err)
			default:
				p.Logger.Error("push failed", "token", short(d.Token), "err", err)
				errs = append(errs, err)
			}
		}()
	}
	wg.Wait()
	return sent, errs
}

func short(token string) string {
	if len(token) > 8 {
		return token[:8] + "…"
	}
	return token
}
