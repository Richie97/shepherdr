// Package pair makes the link and QR code a phone uses to find its hub.
package pair

import (
	"fmt"
	"html/template"
	"net/url"
	"strings"

	"rsc.io/qr"
)

// Link is the app's pairing URL for a hub, e.g.
// shepherdr://pair?hub=https%3A%2F%2Fhub.tailnet.ts.net%3A8443. Scanning it with
// the iPhone Camera opens the app.
func Link(hub string) string {
	return "shepherdr://pair?hub=" + url.QueryEscape(hub)
}

// Terminal renders text as a QR code for a terminal, two modules per
// character row using half blocks, with a quiet zone. Light-on-dark and
// dark-on-light terminals both scan because the colors are explicit.
func Terminal(text string) (string, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 2
	black := func(x, y int) bool {
		x, y = x-quiet, y-quiet
		return x >= 0 && y >= 0 && x < c.Size && y < c.Size && c.Black(x, y)
	}
	n := c.Size + 2*quiet
	var b strings.Builder
	for y := 0; y < n; y += 2 {
		b.WriteString("\x1b[30;47m") // black on white
		for x := 0; x < n; x++ {
			top, bottom := black(x, y), black(x, y+1)
			switch {
			case top && bottom:
				b.WriteString("█")
			case top:
				b.WriteString("▀")
			case bottom:
				b.WriteString("▄")
			default:
				b.WriteString(" ")
			}
		}
		b.WriteString("\x1b[0m\n")
	}
	return b.String(), nil
}

// SVG renders text as a QR code SVG, one unit per module.
func SVG(text string) (template.HTML, error) {
	c, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 4
	n := c.Size + 2*quiet
	var path strings.Builder
	for y := 0; y < c.Size; y++ {
		for x := 0; x < c.Size; x++ {
			if c.Black(x, y) {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	return template.HTML(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges" role="img" aria-label="Pairing QR code"><rect width="100%%" height="100%%" fill="#fff"/><path fill="#000" d="%s"/></svg>`,
		n, n, path.String())), nil
}

var page = template.Must(template.New("pair").Parse(`<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Pair Shepherdr</title>
<style>
:root { color-scheme: light dark; --bg: #f6f6f4; --fg: #1d1d1b; --muted: #6b6b66; --card: #fff; }
@media (prefers-color-scheme: dark) { :root { --bg: #161615; --fg: #ecece8; --muted: #9a9a94; --card: #222220; } }
body { margin: 0; min-height: 100vh; display: grid; place-items: center; background: var(--bg); color: var(--fg);
  font: 16px/1.5 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; }
main { max-width: 26rem; padding: 2rem 1rem; text-align: center; }
.qr { background: #fff; padding: 1rem; border-radius: 16px; width: min(18rem, 80vw); margin: 1.5rem auto; }
.qr svg { display: block; width: 100%; height: auto; }
code { background: var(--card); padding: .15rem .4rem; border-radius: 6px; word-break: break-all; font-size: .85em; }
p { color: var(--muted); }
</style></head>
<body><main>
<h1>Pair Shepherdr</h1>
<p>Point your iPhone camera at the code, or use <b>Scan QR code</b> in the app.</p>
<div class="qr">{{.SVG}}</div>
<p>Hub: <code>{{.Hub}}</code></p>
<p>Your phone needs Tailscale on, signed in as the same user.</p>
</main></body></html>
`))

// Page writes the HTML pairing page for hub.
func Page(w interface{ Write([]byte) (int, error) }, hub string) error {
	svg, err := SVG(Link(hub))
	if err != nil {
		return err
	}
	return page.Execute(w, map[string]any{"SVG": svg, "Hub": hub})
}
