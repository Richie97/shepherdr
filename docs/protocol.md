# Protocol notes

Recorded from herdr-core 0.1.15 on 2026-11-01. The fixtures are in `bridge/testdata/`. When behaviour changes, re-record the fixtures; don't edit them by hand.

## herdr-core socket

- Path: `~/.local/state/herdr-core/<session>/<session>.sock`. The session defaults to `default` and can be set with `HC_SESSION`.
- JSON-RPC 2.0, one JSON object per line, in both directions.
- `hello` is **optional**. Calls work without it. `hello` returns `{core:{id,name}, features, framed, generation, methods, version}`.
- Errors: `{"error":{"code":-32000,"message":"no terminal \"t9999\"","data":{"kind":"not_found","hint":"…"}}}`. Branch on `data.kind`, never on the message text.

### Terminal (`terminal.list` → `result.terminals[]`, also in the subscribe snapshot)

Key fields: `address` (`t1`, or `simrig/t9` for a remote machine), `state` (`live`, …), `status` (`{app, state, kind?, msg?, progress?, title?}` or null for shells), `foreground` (`{argv, cwd, name, pid}`; `argv[0]` is the agent CLI, e.g. `claude`), `cwd`, `title`, `turn`, `name`, `labels`, `exit_code`, `finished_ms`, `created_ms`, `status_ms`.

- `status.state`: `idle | working | done | blocked | error`. `status.kind` is set only when blocked: `permission | question | auth`.
- The Herdr app's own labels: `where` (`"webapp / 1"`, the workspace and tab name), `herdr.space`, `herdr.tab`.
- `terminal.list` also returns `machines[]` (same shape as `machine.list`) and `hidden_finished`.

### `terminal.read`

`{text, complete, how}`. `text` is the screen plus the requested `lines` of history.

### `events.subscribe`

1. The response: `{"id":1,"result":{"channel":null,"seq":N,"terminals":[…]}}`, a full snapshot.
2. Then notifications:
   - `{"method":"event","params":{"type":T,"terminal":"t11","seq":N,"data":{…}}}`
   - `{"method":"heartbeat","params":{"seq":N}}` every 15 s
   - `missed` followed by a fresh snapshot if the subscriber fell behind. Ignore events whose `seq` is not above the snapshot's.

Observed `type` values and their `data`:

| type | data |
|---|---|
| `created` | full Terminal |
| `labels` | `{labels}` (the full new label map) |
| `foreground` | `{argv, cwd, name, pid}` |
| `finished` | `{exit_code, finished_ms, reason, signal}` |
| `removed` | `{}` |
| `status` | `{status: {app, state, kind?, progress?, msg?} or null, reported, records}` |
| `turn` | `{state, turn}`; the turn number goes up each time the status leaves `working` |

Recorded in `events.status.jsonl`: working → blocked/permission → working 50% → done → idle. Documented but not yet recorded: `lost`, `restarted`, `keep`, `title`, `size`, `doc`.

### Status reports (OSC 7501)

A program sets its status by printing `ESC ] 7501 ; :state=<s>:app=<name>[:kind=<k>][:progress=<n>] BEL`. The first field is the record id, empty for the main record. A `msg` containing spaces didn't come through, so don't rely on `msg`.

### Note wrapping

When the sender of `terminal.send` is itself a Herdr terminal (for example an agent using `hc send`) and the target reports a status, the core wraps the text as `<hc from="t3">…</hc>`. Sends through the bridge have no caller terminal and arrive unchanged.

### Framed mode (terminal.attach)

This isn't in the published schema. It was recorded by proxying `hc attach`; the recorder script is described in AGENTS.md.

1. Send the line `{"jsonrpc":"2.0","id":0,"method":"hello","params":{"framed":true}}` and read the reply line. After that, the connection carries only frames.
2. Frame: `channel u32 LE | kind u8 | length u32 LE | payload`.
3. Channel 0, kind 0: a JSON-RPC message. `terminal.attach {terminal, cols, rows, history, observe, display}` returns `{channel, terminal}`.
4. On the stream channel:

| kind | direction | payload |
|---|---|---|
| 0 DATA | both | terminal output, or keystrokes |
| 1 CREDIT | viewer → core | u32: output bytes consumed. Send one per DATA received |
| 2 CLOSE | both | empty: detach, or the stream ended |
| 3 RESIZE | viewer → core | u16 cols, u16 rows |

FOCUS and FACTS exist but weren't observed, so the bridge ignores them. First paint arrives as DATA right after the response.

## Bridge API (v1)

Every request must carry `Tailscale-User-Login` equal to the owner. `tailscale serve` sets this header.

- `GET /v1/health` → `{bridge_version, core: hello-without-methods, machines}`, or 503 with `core_error`.
- `POST /v1/rpc` with `{method, params}` → `{result}`, or `{error: <core error>}` (200).
  - Bridge-side errors use HTTP status codes: 400 `bad_request`, 403 `method_not_allowed`/`forbidden`, 502 `core_unavailable`, each with body `{error:{kind,message}}`.
  - Allowlist: `bridge/internal/core/allow.go`.
- `POST /v1/push/register` with `{token, sandbox, name}` → `{registered, push_enabled}`. `POST /v1/push/unregister` with `{token}`. `POST /v1/push/test` → `{sent, errors}`.
- `/v1/health` also returns `push_enabled`.
- Push payload: `aps` (alert title/subtitle/body, `category` BLOCKED|DONE|ERROR, `thread-id` = address, time-sensitive when blocked) plus `address`, `state`, `app`. `apns-collapse-id` = address, so a newer notification for the same agent replaces the older one.
- `GET /v1/attach?terminal=&cols=&rows=&history=&observe=1`: WebSocket to a live terminal.
  - The first message is text: `{"type":"attached","terminal":{…}}`.
  - Binary messages are terminal bytes in both directions.
  - Text from the client: `{"type":"resize","cols":N,"rows":N}`.
  - The bridge credits the core only after a write to the client succeeds, so a slow phone slows the stream.
  - Closing the socket detaches; the terminal keeps running.
- `GET /v1/ws?terminal=&children_of=&label=k=v`: WebSocket. The first text message is the subscribe response (snapshot); every following message is one core notification line, unchanged.
