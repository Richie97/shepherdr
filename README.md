# Herdr UI

An iPhone app for the coding agents you run in [Herdr](https://herdr.dev): see every agent on every machine, read and drive them, start new ones, and get a push notification (with **Approve** / **Deny**) when one needs you. Everything travels over your own Tailscale network.

- **`bridge/`**: `herdr-bridge`, a small Go service that runs on one "hub" machine. It exposes Herdr's API to your phone through `tailscale serve`, with a method allowlist and an audit log, and sends notifications through APNs.
- **`ios/`**: the SwiftUI app (iOS 18+). The Xcode project is generated with XcodeGen.
- **`docs/`**:
  - **[SETUP.md](docs/SETUP.md)**: how to set it up.
  - [protocol.md](docs/protocol.md): Herdr core and bridge protocol notes.
  - [PLAN.md](docs/PLAN.md): the design.

Contributors and coding agents: read [AGENTS.md](AGENTS.md) first.

MIT licensed; see [LICENSE](LICENSE).
