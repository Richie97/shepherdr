// Package tailnet reads local Tailscale state through the tailscale CLI.
package tailnet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// macAppCLI is where the Mac App Store / standalone Tailscale app keeps its CLI.
const macAppCLI = "/Applications/Tailscale.app/Contents/MacOS/Tailscale"

// CLIPath finds the tailscale CLI: PATH first, then the macOS app bundle.
func CLIPath() (string, error) {
	if p, err := exec.LookPath("tailscale"); err == nil {
		return p, nil
	}
	if _, err := os.Stat(macAppCLI); err == nil {
		return macAppCLI, nil
	}
	return "", errors.New("tailscale CLI not found on PATH or in /Applications/Tailscale.app")
}

// Node is a device on the tailnet.
type Node struct {
	ID       string   `json:"ID"`
	HostName string   `json:"HostName"`
	DNSName  string   `json:"DNSName"`
	OS       string   `json:"OS"`
	Online   bool     `json:"Online"`
	UserID   int64    `json:"UserID"`
	Tags     []string `json:"Tags"`
}

// Status is the subset of `tailscale status --json` the bridge uses.
type Status struct {
	Self Node            `json:"Self"`
	Peer map[string]Node `json:"Peer"`
	User map[string]User `json:"User"`
}

// User is a tailnet user.
type User struct {
	LoginName string `json:"LoginName"`
}

// ParseStatus decodes `tailscale status --json` output.
func ParseStatus(b []byte) (*Status, error) {
	var s Status
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse tailscale status: %w", err)
	}
	return &s, nil
}

// GetStatus runs `tailscale status --json`.
func GetStatus(ctx context.Context) (*Status, error) {
	cli, err := CLIPath()
	if err != nil {
		return nil, err
	}
	out, err := exec.CommandContext(ctx, cli, "status", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status: %w", err)
	}
	return ParseStatus(out)
}

// OwnerLogin is the login name of the user who owns this node, which is what
// `tailscale serve` puts in the Tailscale-User-Login header for that user.
func (s *Status) OwnerLogin() (string, error) {
	u, ok := s.User[fmt.Sprint(s.Self.UserID)]
	if !ok || u.LoginName == "" {
		return "", errors.New("tailscale status has no login for this node's user (tagged node?)")
	}
	return u.LoginName, nil
}

// HostName is this node's MagicDNS name without the trailing dot.
func (s *Status) HostName() string { return strings.TrimSuffix(s.Self.DNSName, ".") }
