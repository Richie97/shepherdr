package push

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testKey(t *testing.T) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	return k, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
}

// fakeAPNs is an HTTP/2 TLS server that checks requests like APNs would.
func fakeAPNs(t *testing.T, pub *ecdsa.PublicKey, status int, reason string, got chan<- *http.Request) *httptest.Server {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Errorf("APNs requires HTTP/2, got %s", r.Proto)
		}
		verifyJWT(t, pub, strings.TrimPrefix(r.Header.Get("authorization"), "bearer "))
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		got <- r
		w.WriteHeader(status)
		if reason != "" {
			json.NewEncoder(w).Encode(map[string]string{"reason": reason})
		}
	}))
	ts.EnableHTTP2 = true
	ts.StartTLS()
	t.Cleanup(ts.Close)
	return ts
}

func verifyJWT(t *testing.T, pub *ecdsa.PublicKey, jwt string) {
	t.Helper()
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt has %d parts", len(parts))
	}
	enc := base64.RawURLEncoding
	var header map[string]string
	hb, _ := enc.DecodeString(parts[0])
	json.Unmarshal(hb, &header)
	if header["alg"] != "ES256" || header["kid"] != "KEY123" {
		t.Errorf("header = %v", header)
	}
	var claims map[string]any
	cb, _ := enc.DecodeString(parts[1])
	json.Unmarshal(cb, &claims)
	if claims["iss"] != "TEAM123" || claims["iat"] == nil {
		t.Errorf("claims = %v", claims)
	}
	sig, _ := enc.DecodeString(parts[2])
	if len(sig) != 64 {
		t.Fatalf("signature is %d bytes, want 64 (raw r||s)", len(sig))
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	r, s := new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])
	if !ecdsa.Verify(pub, sum[:], r, s) {
		t.Error("jwt signature does not verify")
	}
}

func newTestSender(t *testing.T, ts *httptest.Server, pem []byte) *Sender {
	s, err := NewSender(pem, "KEY123", "TEAM123", "dev.example.App")
	if err != nil {
		t.Fatal(err)
	}
	s.client = ts.Client()
	s.hosts = map[bool]string{true: ts.URL, false: ts.URL}
	return s
}

func TestSendBuildsAPNsRequest(t *testing.T) {
	key, pemKey := testKey(t)
	got := make(chan *http.Request, 1)
	ts := fakeAPNs(t, &key.PublicKey, 200, "", got)
	s := newTestSender(t, ts, pemKey)

	n := Message(Alert{Address: "simrig/t9", State: "blocked", Kind: "permission", App: "codex", Where: "track / 1"}, "hub")
	if err := s.Send(context.Background(), "abcd1234", true, n); err != nil {
		t.Fatal(err)
	}
	r := <-got
	if r.URL.Path != "/3/device/abcd1234" {
		t.Errorf("path = %s", r.URL.Path)
	}
	for h, want := range map[string]string{
		"apns-topic": "dev.example.App", "apns-push-type": "alert", "apns-priority": "10", "apns-collapse-id": "simrig/t9",
	} {
		if r.Header.Get(h) != want {
			t.Errorf("%s = %q, want %q", h, r.Header.Get(h), want)
		}
	}
	var p struct {
		APS struct {
			Alert             map[string]string `json:"alert"`
			Category          string            `json:"category"`
			InterruptionLevel string            `json:"interruption-level"`
		} `json:"aps"`
		Address string `json:"address"`
		State   string `json:"state"`
	}
	json.NewDecoder(r.Body).Decode(&p)
	if p.APS.Category != "BLOCKED" || p.APS.InterruptionLevel != "time-sensitive" || p.Address != "simrig/t9" || p.State != "blocked" {
		t.Errorf("payload = %+v", p)
	}
	if p.APS.Alert["title"] != "codex needs you" || p.APS.Alert["subtitle"] != "simrig · track / 1" {
		t.Errorf("alert = %v", p.APS.Alert)
	}
}

func TestSendReportsUnregistered(t *testing.T) {
	key, pemKey := testKey(t)
	for _, tc := range []struct {
		status int
		reason string
	}{{410, "Unregistered"}, {400, "BadDeviceToken"}} {
		got := make(chan *http.Request, 1)
		s := newTestSender(t, fakeAPNs(t, &key.PublicKey, tc.status, tc.reason, got), pemKey)
		err := s.Send(context.Background(), "abcd", false, Notification{Title: "x"})
		if !errors.Is(err, ErrUnregistered) {
			t.Errorf("%d %s: err = %v, want ErrUnregistered", tc.status, tc.reason, err)
		}
	}
}

func TestProviderTokenIsCached(t *testing.T) {
	_, pemKey := testKey(t)
	s, _ := NewSender(pemKey, "KEY123", "TEAM123", "dev.example.App")
	now := time.Unix(1_700_000_000, 0)
	s.now = func() time.Time { return now }
	a, _ := s.providerToken()
	now = now.Add(10 * time.Minute)
	if b, _ := s.providerToken(); a != b {
		t.Error("token refreshed too early")
	}
	now = now.Add(time.Hour)
	if c, _ := s.providerToken(); a == c {
		t.Error("token not refreshed after its lifetime")
	}
}

func TestNewSenderRejectsBadKeys(t *testing.T) {
	_, pemKey := testKey(t)
	if _, err := NewSender([]byte("nope"), "K", "T", "x"); err == nil {
		t.Error("want error for non-PEM key")
	}
	if _, err := NewSender(pemKey, "", "T", "x"); err == nil {
		t.Error("want error for missing key id")
	}
}

func readLines(t *testing.T, path string) [][]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var lines [][]byte
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 16<<20)
	for sc.Scan() {
		lines = append(lines, append([]byte(nil), sc.Bytes()...))
	}
	return lines
}

func TestWatcherAlertsOnRecordedStatusEvents(t *testing.T) {
	w := &Watcher{Cooldown: 20 * time.Second, Logger: slog.Default(), now: time.Now}
	snapshot := readLines(t, "../../testdata/events.subscribe.jsonl")[0]
	if err := w.loadSnapshot(snapshot); err != nil {
		t.Fatal(err)
	}
	// Recorded: working, blocked(permission), working, done, idle (+ turn events).
	var alerts []Alert
	for _, line := range readLines(t, "../../testdata/events.status.jsonl") {
		if a, ok := w.handle(line); ok {
			alerts = append(alerts, a)
		}
	}
	if len(alerts) != 2 {
		t.Fatalf("alerts = %+v, want blocked then done", alerts)
	}
	if alerts[0].State != "blocked" || alerts[0].Kind != "permission" || alerts[1].State != "done" {
		t.Fatalf("alerts = %+v", alerts)
	}
}

func TestWatcherCooldownAndSnapshot(t *testing.T) {
	now := time.Unix(0, 0)
	w := &Watcher{Cooldown: 20 * time.Second, now: func() time.Time { return now }}
	w.loadSnapshot([]byte(`{"id":1,"result":{"terminals":[{"address":"t1","status":{"state":"blocked"}}]}}`))

	ev := func(state string) []byte {
		return []byte(`{"method":"event","params":{"type":"status","terminal":"t1","data":{"status":{"state":"` + state + `"}}}}`)
	}
	if _, ok := w.handle(ev("blocked")); ok {
		t.Error("already blocked in the snapshot: not news")
	}
	w.handle(ev("working"))
	if _, ok := w.handle(ev("blocked")); !ok {
		t.Error("working -> blocked should alert")
	}
	w.handle(ev("working"))
	now = now.Add(5 * time.Second)
	if _, ok := w.handle(ev("blocked")); ok {
		t.Error("repeat within cooldown should be suppressed")
	}
	w.handle(ev("working"))
	now = now.Add(30 * time.Second)
	if _, ok := w.handle(ev("blocked")); !ok {
		t.Error("after cooldown should alert again")
	}
}

func TestMessageFallbacks(t *testing.T) {
	n := Message(Alert{Address: "t3", State: "done", Name: "reviewer", Title: "Review PR 12"}, "simrig")
	if n.Title != "reviewer finished" || n.Subtitle != "simrig" || n.Body != "Review PR 12" || n.TimeSensitive {
		t.Errorf("done = %+v", n)
	}
	n = Message(Alert{Address: "mac/t1", State: "blocked", Kind: "question", Msg: "Which branch?"}, "simrig")
	if n.Title != "mac/t1 needs you" || n.Body != "Which branch?" || n.Subtitle != "mac" {
		t.Errorf("blocked = %+v", n)
	}
	n = Message(Alert{Address: "t2", State: "error", App: "codex", Cwd: "C:\\src"}, "simrig")
	if n.Title != "codex hit an error" || n.Body != "C:\\src" || n.Category != "ERROR" {
		t.Errorf("error = %+v", n)
	}
}

func TestDevicesPersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "devices.json")
	d, err := OpenDevices(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.Add(Device{Token: "not hex!"}); err == nil {
		t.Error("want error for non-hex token")
	}
	tok := strings.Repeat("ab", 32)
	d.Add(Device{Token: tok, Sandbox: true, Name: "iPhone"})
	d.Add(Device{Token: tok, Sandbox: false, Name: "iPhone"}) // update, not duplicate

	d2, err := OpenDevices(path)
	if err != nil {
		t.Fatal(err)
	}
	if l := d2.List(); len(l) != 1 || l[0].Sandbox {
		t.Fatalf("reloaded = %+v", l)
	}
	d2.Remove(tok)
	d3, _ := OpenDevices(path)
	if len(d3.List()) != 0 {
		t.Fatal("remove not persisted")
	}
}
