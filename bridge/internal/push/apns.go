// Package push sends APNs notifications when agents need attention.
package push

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	hostProduction = "https://api.push.apple.com"
	hostSandbox    = "https://api.sandbox.push.apple.com"
	// Apple rejects provider tokens older than an hour and throttles
	// refreshing more than every 20 minutes.
	tokenLifetime = 40 * time.Minute
)

// ErrUnregistered means the device token is no longer valid; forget it.
var ErrUnregistered = errors.New("apns: device token is no longer valid")

// Sender sends notifications with token-based (.p8) APNs auth.
type Sender struct {
	KeyID  string
	TeamID string
	// Topic is the app's bundle ID.
	Topic string

	key    *ecdsa.PrivateKey
	client *http.Client
	// hosts maps sandbox=true/false to a base URL; overridable in tests.
	hosts map[bool]string
	now   func() time.Time

	mu       sync.Mutex
	jwt      string
	jwtIssue time.Time
}

// NewSender parses a .p8 key (PKCS#8 PEM, as downloaded from Apple).
func NewSender(p8 []byte, keyID, teamID, topic string) (*Sender, error) {
	block, _ := pem.Decode(p8)
	if block == nil {
		return nil, errors.New("apns: key is not PEM")
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("apns: parse key: %w", err)
	}
	ec, ok := k.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("apns: key is not an EC key")
	}
	if keyID == "" || teamID == "" || topic == "" {
		return nil, errors.New("apns: key_id, team_id and topic are required")
	}
	return &Sender{
		KeyID: keyID, TeamID: teamID, Topic: topic,
		key:    ec,
		client: &http.Client{Timeout: 15 * time.Second},
		hosts:  map[bool]string{false: hostProduction, true: hostSandbox},
		now:    time.Now,
	}, nil
}

// providerToken returns a cached ES256 JWT, refreshing it when old.
func (s *Sender) providerToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if s.jwt != "" && now.Sub(s.jwtIssue) < tokenLifetime {
		return s.jwt, nil
	}
	enc := base64.RawURLEncoding
	header, _ := json.Marshal(map[string]string{"alg": "ES256", "kid": s.KeyID})
	claims, _ := json.Marshal(map[string]any{"iss": s.TeamID, "iat": now.Unix()})
	input := enc.EncodeToString(header) + "." + enc.EncodeToString(claims)
	sum := sha256.Sum256([]byte(input))
	r, ss, err := ecdsa.Sign(rand.Reader, s.key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64) // JWS wants raw r||s, each left-padded to 32 bytes
	r.FillBytes(sig[:32])
	ss.FillBytes(sig[32:])
	s.jwt = input + "." + enc.EncodeToString(sig)
	s.jwtIssue = now
	return s.jwt, nil
}

// Notification is one alert.
type Notification struct {
	Title    string
	Subtitle string
	Body     string
	// Category selects the app's notification actions (BLOCKED, DONE, ERROR).
	Category string
	// ThreadID groups notifications; CollapseID replaces an older one.
	ThreadID   string
	CollapseID string
	// TimeSensitive breaks through Focus (for blocked agents).
	TimeSensitive bool
	// Data is merged into the payload for the app (address, state, …).
	Data map[string]string
}

func (n Notification) payload() ([]byte, error) {
	aps := map[string]any{
		"alert":     map[string]string{"title": n.Title, "subtitle": n.Subtitle, "body": n.Body},
		"sound":     "default",
		"category":  n.Category,
		"thread-id": n.ThreadID,
	}
	if n.TimeSensitive {
		aps["interruption-level"] = "time-sensitive"
	}
	p := map[string]any{"aps": aps}
	for k, v := range n.Data {
		p[k] = v
	}
	return json.Marshal(p)
}

// Send delivers n to one device. sandbox selects the development gateway,
// which is what Xcode-installed builds use.
func (s *Sender) Send(ctx context.Context, deviceToken string, sandbox bool, n Notification) error {
	body, err := n.payload()
	if err != nil {
		return err
	}
	jwt, err := s.providerToken()
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.hosts[sandbox]+"/3/device/"+deviceToken, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("authorization", "bearer "+jwt)
	req.Header.Set("apns-topic", s.Topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("apns-expiration", fmt.Sprint(s.now().Add(time.Hour).Unix()))
	if n.CollapseID != "" {
		req.Header.Set("apns-collapse-id", truncate(n.CollapseID, 64))
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("apns: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	var r struct {
		Reason string `json:"reason"`
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	json.Unmarshal(b, &r)
	switch {
	case resp.StatusCode == http.StatusGone,
		r.Reason == "BadDeviceToken", r.Reason == "Unregistered", r.Reason == "DeviceTokenNotForTopic":
		return fmt.Errorf("%w (%d %s)", ErrUnregistered, resp.StatusCode, r.Reason)
	}
	return fmt.Errorf("apns: %d %s", resp.StatusCode, r.Reason)
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
