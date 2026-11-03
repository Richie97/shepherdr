# AGENTS.md

How we work on **herdr-ui**: an iOS app plus a Go bridge for controlling Herdr agents over Tailscale. Every agent working in this repo (Claude Code, Codex, …) reads this first.

## Source of truth

- `docs/PLAN.md`: architecture, bridge API, phases. If a decision changes the plan, update PLAN.md in the same change.
- `docs/protocol.md`: the herdr-core payloads we depend on, written from recorded fixtures, not from memory.
- `## Decisions` below: a short log of choices and why we made them.
- `## Status` below: the current phase and what comes next. Update it when a phase step lands.

## Ground truth about Herdr

- We target **herdr-core** (`hc`). It's JSON-RPC 2.0, one message per line, on `~/.local/state/herdr-core/default/default.sock`. We do not target the old `herdr` TUI server (`~/.config/herdr/herdr.sock`).
- The installed binary has the final say: `herdr-core schema`, `hc <cmd> --help`, `hc --skill`. Check against these before assuming a method, parameter or field exists.
- Terminal addresses can be remote: `simrig/t9`. Never assume `t*` is local.

## Safety rules for the live core

The core on this Mac runs the user's real agents.

- **Read-only by default.** `terminal.list/get/read`, `events.subscribe`, `machine.list`, `templates.list` and `hello` are always safe.
- **Ask first** before anything that types into, starts, kills or removes a real terminal: `terminal.send/keys/interrupt/kill/rm/run`, and anything under `machine.*` that changes state.
- For mutation tests, start a throwaway shell yourself (`hc run --bg --name scratch-… zsh`), work only on that, and remove it afterwards.
- Never call `server.stop/restart`, `machine.rm` or `doc.set` on the live core.
- The bridge allowlist in `internal/core` exists for safety. Widen it on purpose, in its own commit, with a reason.

## Workflow

- Work one phase step at a time from PLAN.md. Keep changes small and reviewable.
- **Fixtures first:** capture real payloads into `bridge/testdata/` before writing parsing code, and test against them. If a payload shape changes, re-record the fixture rather than editing it by hand.
- Run the relevant tests and build before calling something done, and report real results, including failures.
- **Commits:** imperative subject, a body if useful, ending with the agent's `Co-Authored-By:` line. Commits are signed through 1Password. If signing fails, stop and tell the user; never use `--no-gpg-sign` or `-c commit.gpgsign=false`.
- **Every change goes through a pull request.** Never commit, merge or push directly to `main`, not even a fast-forward:
  1. Work on a branch (or a worktree's branch) cut from an up-to-date `origin/main`.
  2. When the change is done and tested locally, push the branch and open a PR with `gh pr create`. The PR description says what changed, why, and how it was tested.
  3. Merge only after CI (`ci.yml`) passes on the PR **and** the user has said to merge. Use `gh pr merge`, then delete the branch.
  
  Merging to `main` is a release step: when `ios/` or `appstore/` changed, it uploads to TestFlight and syncs the App Store listing.
- Pushing a feature branch to open its PR is part of the workflow. Anything else needs the user to ask first: pushing `main`, tagging (including moving `testflight-uploaded` or `appstore-synced`), or publishing.

## Bridge (Go): `bridge/`

- Go (latest stable), standard library first. Dependencies: `github.com/coder/websocket` and `rsc.io/qr`. APNs is standard library only. Ask before adding more.
- Layout: `cmd/herdr-bridge`, `internal/{core,server,push,tailnet}`.
- Listens on `127.0.0.1` only. Exposure to the tailnet is always through `tailscale serve`. Every request checks `Tailscale-User-Login`.
- Tailscale CLI path: `/Applications/Tailscale.app/Contents/MacOS/Tailscale` on macOS, otherwise `tailscale` from PATH.
- Commands: `go test ./...`, `go vet ./...`, `gofmt -l .` must print nothing.
- Never commit secrets: no APNs `.p8` keys, tokens or tailnet auth keys.
- Push config lives in the OS user config dir (`%AppData%\herdr-bridge\` on simrig, `~/Library/Application Support/herdr-bridge/` on macOS):
  - `apns.json`: `{key_id, team_id, topic}`
  - `apns.p8`: the key
  - `devices.json`: registered phones
  Without `apns.json` the bridge still accepts registrations and logs alerts.
- Testing status without a real agent: a script that prints `\033]7501;:state=blocked:app=x:kind=permission\007` sets the terminal's status. Start it with `hc run --bg --no-notify`, or the core types `<hc>` notes into this session's terminal. Don't `hc send` from here to a terminal that reports status: the core wraps the text in `<hc from=…>` tags. Sends through the bridge are not wrapped.

## iOS app: `ios/`

- SwiftUI, iOS 18+, Swift 6 strict concurrency, Observation (`@Observable`). The only third-party dependency is SwiftTerm (live terminal), which pulls in a build plugin. Pass `-skipPackagePluginValidation` to `xcodebuild`; in Xcode, accept "Trust & Enable" once.
- The project is generated by XcodeGen from `ios/project.yml`. Commit `project.yml`; don't hand-edit or commit the `.xcodeproj`.
- Model every identifier as `(machine, address)`.
- iPhone Duo (foldable, iOS 27.1+): the outer screen is 466×678 pt (compact) and the inner one 669×951 pt. `AgentsView` switches on `horizontalSizeClass`: a stack when compact, a list + detail `NavigationSplitView` when regular. Both share one `selected` agent, so folding keeps it open.
  - This Mac has no Simulator.app, so the Duo can't be unfolded here. Its outer screen is `simctl io <duo> screenshot --display=1`.
  - Test the regular-width layout with `-demo YES -landscape YES` on an iPhone 17 Pro Max.
  - Keep the agent list's `List(selection:)` bound to `selected` in both layouts. A constant binding makes taps on `NavigationLink(value:)` do nothing on phones. `ShepherdrUITests` taps through the demo agents to catch this.
- TestFlight: `ios/scripts/testflight.sh` (`--no-upload` only archives). It signs and exports the `.ipa` locally, then uploads it with `testflight_distribute.py --upload` through the App Store Connect **Build Uploads API**. xcodebuild's own upload rejects team API keys ("Cannot verify client's JWT headers", CDWebService 3004) even when the key works for the API and for signing. It uses a UTC timestamp build number; bump `MARKETING_VERSION` in project.yml for a new version. Uploading needs an Xcode account or `ASC_KEY_PATH`/`ASC_KEY_ID`/`ASC_ISSUER_ID`.
- CI (`.github/workflows`):
  - `ci.yml`: bridge and iOS tests on pull requests and on pushes to main. This is the only place tests run.
  - `testflight.yml`: starts when CI completes on main (`workflow_run`). It uploads only if CI passed and `ios/` changed since the `testflight-uploaded` tag, then moves the tag. A manual run always uploads. Needs the secrets `ASC_KEY_ID`, `ASC_ISSUER_ID` and `ASC_KEY_P8`.
  - After uploading, the `distribute` job (Linux) runs `ios/scripts/testflight_distribute.py`, using only the standard library and `openssl`. It waits for Apple's processing, sets "What to Test" from the commit subjects since the last upload, and adds the build to the TestFlight group **Internal** (skipped if that group has automatic distribution on).
  - The same workflow's `store` job runs `ios/scripts/appstore_sync.py` when `appstore/` or `ios/project.yml` changed since the tag `appstore-synced`. It pushes the listing text, categories and screenshots, creates or renames the version to `MARKETING_VERSION`, and never submits for review. `appstore/metadata/` is the source of truth; edits made in App Store Connect get overwritten.
  - Workflow changes to `workflow_run` workflows only take effect once they're on main.
  - Both use macOS 26 runners with the latest stable Xcode, because SwiftTerm needs Swift 6.2.
- Generate the project: `cd ios && xcodegen generate`. Test: `xcodebuild -scheme Shepherdr -destination 'platform=iOS Simulator,name=iPhone 17' -derivedDataPath build -skipPackagePluginValidation test`. Tests decode the fixtures in `bridge/testdata`, which are bundled as the `testdata` folder.
- Bundle ID `app.shepherdr`, team `L3NS86NMXZ`. URL scheme: `shepherdr://terminal/<address>`.
- The simulator uses the Mac's tailnet, so it can talk to the live hub. Set the hub without the UI: `xcrun simctl spawn <sim> defaults write app.shepherdr hubURL https://simrig.<tailnet>.ts.net:8443`.
- Screenshot an agent (Debug builds only): `xcrun simctl launch <sim> app.shepherdr -openTerminal t9`, then `xcrun simctl io <sim> screenshot out.png`. Add `-openLive YES` to go straight to its live terminal.
- Live terminal: an attached viewer that types or resizes sets the terminal's size for everyone, Herdr desktop included, until another viewer types. "Watch only" attaches with `observe=1` and never resizes.
- Framed protocol recorder: proxy `~/.local/state/herdr-core/sniff/sniff.sock` to the real socket, then run `HC_SESSION=sniff hc attach <scratch>` in a pty. Notes are in docs/protocol.md.
- Addresses are relative to the hub's core. On simrig, its own terminals are `t9` and the Mac's are `erics-macbook-pro/t3`.
- The app refetches a terminal (`terminal.get`) on every event except `created`, `removed`, `size` and `doc`, so it doesn't depend on each event's data shape. Screen output isn't an event, so the detail screen polls every 2 s while the agent is working.

## Decisions

- 2026-11-01: Target herdr-core rather than the herdr TUI server, because the Herdr app uses it and it handles multiple machines.
- 2026-11-01: Run one bridge on the hub Mac instead of one per box, because the core already reaches other machines over SSH.
- 2026-11-01: Use Tailscale for transport and identity (`tailscale serve` + `Tailscale-User-Login`). No custom auth.
- 2026-11-01: The bridge is written in Go. Push goes through APNs from the bridge (the user has an Apple Developer account).
- 2026-11-01: The hub is **simrig** (Windows, always on), not the Mac. simrig's core has the Mac added, so it sees every terminal, and it stays up when the laptop sleeps.
- 2026-11-01: On Windows the bridge reaches the core with `hc connect` (stdin/stdout), because there's no Unix socket there. It starts at logon from the `HKCU\…\Run` key, and `install` starts it immediately through Task Scheduler, because `hc exec` and ssh job objects kill processes started directly. No admin is needed.
- 2026-11-01: Push goes out when an agent's status *changes into* blocked, done or error, with a 20 s cooldown per terminal+state. States already in the snapshot when the bridge starts don't notify. Blocked notifications are time-sensitive.
- 2026-11-01: The notification Approve button presses `1` for claude-code, `y` for codex and Enter for anything else; Deny presses Esc. Both require an unlocked phone, and both re-check that the agent is still blocked first.
- 2026-11-01: The phone finds the hub by scanning a QR code once.
- 2026-11-01: The app is named **Shepherdr**: bundle ID `app.shepherdr`, URL scheme `shepherdr://`, Xcode target and scheme `Shepherdr`. The repo, `herdr-bridge` and the Go module keep their names. Renaming changed the APNs topic (simrig's `apns.json`); old device tokens get dropped automatically (DeviceTokenNotForTopic). Machines come from the core's machine list, and the bridge suggests tailnet peers that aren't added yet.
- 2026-11-02: The app supports iPad (`TARGETED_DEVICE_FAMILY "1,2"`, all four orientations for multitasking), using the same regular-width split as the unfolded Duo. Once a version with iPad support is released it can't be dropped. The store listing carries 13" iPad screenshots in `appstore/screenshots-ipad/`.
- 2026-11-02: Send with an empty composer accepts the agent's suggestion: Tab, a 300 ms pause, then Enter, as two `terminal.keys` calls. One call can deliver them 1 ms apart. It asks for confirmation when the agent is blocked, like any send.

## Status

- Phase 0 (fixtures): **done**. Fixtures are in `bridge/testdata/`; notes in `docs/protocol.md`.
- Phase 1 (bridge core): **mostly done**. `/v1/health`, `/v1/rpc` (allowlist), `/v1/ws`, the identity check and `herdr-bridge install` are built, tested, and smoke-tested against the live core on 127.0.0.1:7799.
  - Deployed to simrig at `%LOCALAPPDATA%\Programs\herdr-bridge\` (log `herdr-bridge.log` next to it). Running, and verified locally on simrig.
  - Live at `https://simrig.<tailnet>.ts.net:8443`. Checked from the Mac over the tailnet: `/v1/health` and `/v1/rpc terminal.list` return terminals on both machines. A forged `Tailscale-User-Login` header is replaced by `tailscale serve` with the caller's real identity.
  - The very first HTTPS request after enabling Serve fails while the certificate is issued. Retry after about 10 seconds.
  - Redeploy: `bridge/scripts/deploy-windows.sh simrig`. It uses only Herdr's link, not ssh, which fails whenever 1Password's agent is locked:
    1. Build a stripped exe.
    2. Upload it in chunks with `hc file write` (8 MB limit per write) and join them on simrig.
    3. Check the hash, then run `herdr-bridge.new.exe install`, which stops the old bridge and copies itself into place.
- Phase 2 (iOS MVP): **done**.
  - Built: connect screen, agents list grouped by state across machines, agent detail (screen, prompt composer, quick keys, Stop), settings, `shepherdr://` links. 9 unit tests pass.
  - Checked in the simulator against the live simrig hub. Send and keys checked through the bridge on a scratch shell.
- Phase 3 (push): **done**, tested on a real iPhone 16 Pro (development push service).
  - The APNs key `2R3UG5SJ28` is on simrig, outside the repo.
  - Notifications arrive for blocked, done and error, plus the Settings test button.
  - Verified on a scratch agent, by comparing the bridge's `input` log with the keys the agent received: Approve pressed `enter` and Deny pressed `esc`.
  - Not yet checked against a real Claude Code or Codex permission dialog (Approve presses `1` and `y` there).
  - Every input the bridge forwards is logged in `herdr-bridge.log` as `msg=input` (method, terminal, keys, text length). Check that log first when "who typed this?" comes up.
- Phase 5 (live terminal, new agents, pairing): **built**, verified in the simulator and against simrig. Not yet tried on the iPhone.
  - Live terminal: SwiftTerm over `/v1/attach`, using the framed protocol (docs/protocol.md).
  - New agent: `/v1/start` runs a kept shell labeled `herdr.space`/`herdr.tab` and types the agent command, the same way the Herdr app does. A task is delivered once the agent reports idle.
  - Pairing: `herdr-bridge pair` (terminal QR), the `/pair` page, `shepherdr://pair?hub=` links, and the in-app scanner (VisionKit). The QR encoder is `rsc.io/qr`, approved by the user.
