package pair

import (
	"net/url"
	"strings"
	"testing"
)

func TestLinkRoundTrips(t *testing.T) {
	hub := "https://hub.example.ts.net:8443"
	l := Link(hub)
	u, err := url.Parse(l)
	if err != nil || u.Scheme != "herdr" || u.Host != "pair" || u.Query().Get("hub") != hub {
		t.Fatalf("Link = %q (%v)", l, err)
	}
}

func TestTerminalQRIsSquareWithQuietZone(t *testing.T) {
	s, err := Terminal(Link("https://hub.example.ts.net:8443"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	width := len([]rune(strings.TrimSuffix(strings.TrimPrefix(lines[0], "\x1b[30;47m"), "\x1b[0m")))
	if width < 25 || len(lines) != (width+1)/2 {
		t.Fatalf("width %d, %d lines", width, len(lines))
	}
	if !strings.Contains(s, "█") || strings.Trim(strings.TrimSuffix(strings.TrimPrefix(lines[0], "\x1b[30;47m"), "\x1b[0m"), " ") != "" {
		t.Error("first row should be quiet zone, and the code should have dark modules")
	}
}

func TestPageHasQRAndHub(t *testing.T) {
	var b strings.Builder
	if err := Page(&b, "https://hub.example.ts.net:8443"); err != nil {
		t.Fatal(err)
	}
	h := b.String()
	for _, want := range []string{"<svg", "hub.example.ts.net:8443", "<title>Pair Herdr</title>"} {
		if !strings.Contains(h, want) {
			t.Errorf("page missing %q", want)
		}
	}
}
