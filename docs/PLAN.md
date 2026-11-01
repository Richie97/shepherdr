# Shepherdr: iOS remote for Herdr agents

An iOS app that lets you see and control the coding agents running in Herdr on each of your machines. All traffic goes over Tailscale.

## What Herdr actually is here

The Herdr desktop app (`/Applications/Herdr.app`) runs on **herdr-core** (`hc`, v0.1.15, `~/.local/bin/herdr-core`). It does not use the older `herdr` TUI server (`~/.config/herdr/herdr.sock`, protocol 22), which isn't running. This project ignores that server.

- **API:** JSON-RPC 2.0, one message per line, on `~/.local/state/herdr-core/default/default.sock`. `hc connect` speaks the same protocol over stdin/stdout. `rpc.discover` and `hc schema` return the full OpenRPC document.
- **Multi-machine support is built in.** The core dials other machines over SSH (`machine.add/list/connect`). Their terminals show up in `terminal.list` and `events.subscribe` under addresses like `simrig/t9`. `simrig` (Windows) is already connected.
- **Agent status:** `idle | working | done | blocked | error`. A `blocked` status also carries `kind` (`permission`, `question` or `auth`), `msg` and `progress`.
- Each terminal also carries `agent` (`claude-code`, `codex`, …), `cwd`, `turn`, `name`, `labels`, and `where` (`"webapp / 1"`, the app's workspace and tab).

Relevant methods:

| Method | Use in the app |
|---|---|
| `events.subscribe` | Initial snapshot of every terminal on every machine, then every change in order (`seq`, plus a `missed` resync) |
| `terminal.list/get` | Agents list and detail |
| `terminal.read` | Screen text plus N lines of history |
| `terminal.send` | Send a prompt: pastes the text and presses Enter once nobody has typed for about 3 seconds |
| `terminal.keys` | Quick keys: enter, esc, up, ctrl+c… |
| `terminal.interrupt` | Stop button |
| `terminal.attach` (framed) | Raw live stream of a terminal, for a full terminal view |
| `terminal.put` | Upload a photo or screenshot beside an agent |
| `terminal.run` / `templates.list` | Start a new agent (e.g. `claude`, or a named template with a task) on any machine (`on`) |
| `machine.list/add/connect` | Machines screen |

## Architecture

```
iPhone (SwiftUI)
  │  HTTPS + WebSocket over the tailnet
  ▼
tailscale serve --https=8443  ─►  herdr-bridge (127.0.0.1:7777)  ─►  hc connect / core socket
  (TLS, Tailscale-User-Login header)   (Go daemon, on simrig)             └─ ssh ─► erics-macbook-pro, other boxes
```

- **One bridge per hub, not one per box.** The hub is **simrig** (Windows, always on). Its core has the Mac added, so it sees every terminal (`erics-macbook-pro/t3`, …) and stays up when the laptop sleeps. The same binary also runs on macOS, over the Unix socket, if we ever want a second hub.
- **"Discovery" comes mostly for free:**
  - The machine list is whatever the core has added.
  - The bridge also reads `tailscale status --json` and suggests online peers that aren't added yet. "Add" calls `machine.add`.
  - The phone finds the hub by scanning a QR code from `herdr-bridge pair` once. It could later also try every bridge-tagged peer on 8443.
- **Auth:** the bridge accepts only requests whose `Tailscale-User-Login` matches the owner. It keeps an allowlist of methods and never forwards `server.stop/restart`, `file.*`, `machine.exec` or `doc.set`.
- When the Mac sleeps, its terminals show as disconnected. simrig's agents and push notifications keep working.

## Bridge API (v1)

| Endpoint | Purpose |
|---|---|
| `GET /v1/health` | Bridge and core version, core identity, machine states |
| `GET /v1/machines` | Machines the core has added, plus tailnet peers not added yet |
| `POST /v1/rpc` | Forwards one allowlisted JSON-RPC call |
| `GET /v1/ws` | Event stream: `events.subscribe` snapshot and changes, fanned out to every connected phone |
| `GET /v1/attach/{t}` | WebSocket that proxies `terminal.attach` (framed) for a live terminal view |
| `POST /v1/push/register` | Saves the device's APNs token |

## Push notifications (APNs, token auth, .p8 key on the hub)

- The bridge keeps its own `events.subscribe` stream open at all times, whether or not a phone is connected.
- It sends a push in these cases:
  - Status changes to **`blocked`**. The `kind` and `msg` go in the body, e.g. "codex on simrig wants permission: run `npm test`?". The notification gets **Approve**/**Deny** buttons that send the right keys.
  - Status changes to **`done`** or **`error`**.
- Pushes are grouped per terminal, with a short cooldown so status flapping doesn't spam you.
- Later: a Live Activity showing "3 working · 1 blocked" plus progress.

## iOS app (SwiftUI, iOS 18+, Swift 6)

- **Agents**, the home screen: agents from every machine, grouped Blocked → Done/Error → Working → Idle. Each row shows a machine badge, agent kind, `where`, cwd and status message. Shells (`zsh`, `powershell.exe`) are hidden by default.
- **Agent detail:**
  - Version 1: screen text from `terminal.read`, refreshed on events.
  - Version 2: a live SwiftTerm view over `/v1/attach`.
  - Prompt composer (`terminal.send`) with dictation and saved snippets, a photo attachment (`terminal.put`), a quick-key bar (Esc, ^C, Enter, ↑/↓, y/n, 1/2/3) and a Stop button (`terminal.interrupt`).
- **New agent:** choose machine, cwd and agent or template, plus an optional task. Calls `terminal.run --bg`.
- **Machines:** core machines with their connection state, a reconnect button, and suggested tailnet peers.

## Phases

0. **Fixtures**: record real `events.subscribe`, `terminal.read` and `hello` payloads from the live core into `bridge/testdata`.
1. **Bridge core**: socket client (JSON-RPC over lines), `/health`, `/rpc` with allowlist, `/ws` fan-out, the Tailscale identity check, and `herdr-bridge install` (launchd plus `tailscale serve`).
2. **iOS MVP**: QR pairing, the agents dashboard, agent detail (read, send, keys, interrupt).
3. **Push**: APNs from the bridge, notification actions for blocked agents.
4. **Live terminal**: `/v1/attach` proxy, SwiftTerm view.
5. **Polish**: new-agent flow, machines and tailnet suggestions, photo upload, widgets and Live Activity, watchOS approve.

## Repo layout

```
bridge/   Go module (cmd/herdr-bridge, internal/{core,server,push,tailnet})
ios/      XcodeGen project.yml + Shepherdr app
docs/     PLAN.md, protocol.md
```

## Setup notes

- Go isn't installed yet: run `brew install go`.
- The Tailscale CLI is at `/Applications/Tailscale.app/Contents/MacOS/Tailscale`, not on PATH.
- Tailnet: `erics-macbook-pro`, `simrig`, `iphone171` (`<tailnet>.ts.net`).
