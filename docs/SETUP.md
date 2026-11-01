# Setting up Shepherdr

Shepherdr is an iPhone app for watching and steering the coding agents you run in [Herdr](https://herdr.dev), on all your machines. It talks to a small bridge that runs on one machine, over your Tailscale network. Nothing is exposed to the internet.

```
iPhone ──Tailscale (HTTPS)──► herdr-bridge on your "hub" ──► Herdr core on the hub ──► your other machines
```

What you get:
- Every agent on every machine, grouped by what needs you.
- Each agent's screen, a live terminal, prompts and quick keys.
- Starting new agents from the phone.
- Push notifications when an agent is blocked, finishes or errors, with **Approve** / **Deny** right on the notification.

About 30 minutes end to end. Push notifications are optional.

---

## 1. What you need

| | |
|---|---|
| **Herdr** | The Herdr desktop app, or `hc` / herdr-core 0.1.15+, on each machine that runs agents. |
| **Tailscale** | On every machine and on the iPhone, all signed in as **the same Tailscale user**. Turn on MagicDNS and HTTPS certificates for your tailnet: admin console → DNS. |
| **A hub** | One machine that is usually on: a desktop, home server or always-on Windows box. macOS and Windows are supported directly. On Linux, run the bridge under systemd (see 3c). |
| **To build** | Go 1.23+ for the bridge. A Mac with Xcode 26+ (SwiftTerm needs Swift 6.2) and [XcodeGen](https://github.com/yonaskolb/XcodeGen) (`brew install xcodegen`) for the app. |
| **Push (optional)** | A paid Apple Developer account. A free account can still install the app on your own phone, but without push. |

## 2. Connect your machines in Herdr

The bridge only talks to the Herdr core on the hub. That core must be able to see your other machines.

```sh
# on the hub
hc machine add you@laptop      # once per other machine (ssh target)
hc ls                          # should list terminals from every machine, e.g. laptop/t3
```

If you add the machines from the Herdr desktop app instead, check with `hc machine ls`.

## 3. Install the bridge on the hub

Clone this repo on a machine with Go:

```sh
git clone https://github.com/Richie97/herdr-ui && cd herdr-ui/bridge
go test ./...
```

### 3a. macOS hub

```sh
go build -o ~/.local/bin/herdr-bridge ./cmd/herdr-bridge
~/.local/bin/herdr-bridge install
```

`install` sets up a launchd agent (`~/Library/LaunchAgents/dev.herdr-ui.bridge.plist`, log at `~/Library/Logs/herdr-bridge.log`) and runs `tailscale serve --bg --https=8443`.

### 3b. Windows hub

**Option 1: on the Windows machine** (with Go installed):

```powershell
cd herdr-ui\bridge
go build -trimpath -ldflags "-s -w -H=windowsgui" -o herdr-bridge.new.exe .\cmd\herdr-bridge
.\herdr-bridge.new.exe install
```

**Option 2: from a Mac or Linux machine whose Herdr core has added the Windows machine.** No ssh is needed:

```sh
bridge/scripts/deploy-windows.sh <machine-name>    # as listed by `hc machine ls`
```

Either way, the bridge ends up in `%LOCALAPPDATA%\Programs\herdr-bridge\` with its log beside it. It starts at every logon from your user's Run key, so no admin rights are needed, and it reaches Herdr through `hc connect`. Run the same command again to upgrade.

### 3c. Linux hub

`install` doesn't support Linux yet. Run the bridge as a systemd user service:

```ini
# ~/.config/systemd/user/herdr-bridge.service
[Service]
ExecStart=%h/.local/bin/herdr-bridge serve
Restart=always
[Install]
WantedBy=default.target
```

```sh
systemctl --user enable --now herdr-bridge
tailscale serve --bg --https=8443 http://127.0.0.1:7777
```

### First run: enable Tailscale Serve

The first time, `tailscale serve` stops and prints a link like `https://login.tailscale.com/f/serve?node=…`. Open it, enable Serve for your tailnet, then run `install` again.

### Check it

On any device on your tailnet, open:

```
https://<hub>.<your-tailnet>.ts.net:8443/v1/health
```

You should see JSON with your core's name and machines. **The very first request can fail for about 10 seconds** while Tailscale issues the HTTPS certificate.

## 4. Build the iPhone app

The project is generated from `ios/project.yml`. Point it at your own Apple account first:

1. In `ios/project.yml`, set:
   - `DEVELOPMENT_TEAM` to your Team ID (developer.apple.com → Membership)
   - `bundleIdPrefix` and `PRODUCT_BUNDLE_IDENTIFIER` to something you own, e.g. `com.yourname.Shepherdr`
2. Generate and open the project:
   ```sh
   cd ios
   xcodegen generate
   open Shepherdr.xcodeproj
   ```
3. Choose your iPhone as the run destination and press **Run**.
   - When Xcode asks about the SwiftTerm package plugin, choose **Trust & Enable**.
   - If the phone says the developer isn't trusted: Settings → General → VPN & Device Management → your profile → **Trust**.

Command-line build, if you prefer:

```sh
xcodebuild -scheme Shepherdr -destination 'id=<device-udid>' -allowProvisioningUpdates -skipPackagePluginValidation build
```

### TestFlight (optional)

To install through TestFlight instead of Xcode:

1. **Create the app record:** App Store Connect → Apps → **+** → New App. Choose iOS, your bundle ID, any name and SKU.
2. **Give the upload tools access** in one of two ways:
   - Sign in to Xcode with your Apple ID (Xcode → Settings → Accounts).
   - Or create an App Store Connect API key (Users and Access → Integrations → Team Keys, role App Manager) and export `ASC_KEY_PATH`, `ASC_KEY_ID` and `ASC_ISSUER_ID`.
3. **Upload:** run `ios/scripts/testflight.sh`. It archives a Release build with a timestamp build number and uploads it. Use `--no-upload` to only archive.
   - **CI instead:** add the API key as repository secrets `ASC_KEY_ID`, `ASC_ISSUER_ID` and `ASC_KEY_P8` (the file's full contents). Then CI tests every push, and when CI passes on `main` with changes under `ios/` since the last upload, `.github/workflows/testflight.yml` archives and uploads a build. You can also run it on demand from the Actions tab. Give the key the **Admin** role, or App Manager with access to certificates, so CI can sign through Apple's cloud signing.
4. **Install:** in App Store Connect → TestFlight, add yourself to an **internal** testing group and install from the TestFlight app.

Notes:
- External testers need Beta App Review, and reviewers can't reach your tailnet.
- TestFlight builds use Apple's production push service. The same `.p8` key works, and the app tells the bridge which service to use.

## 5. Pair the phone

With Tailscale on, on the phone:

- **Easiest:** open `https://<hub>…:8443/pair` in a desktop browser, or run `herdr-bridge pair` on the hub. Point the iPhone **Camera** at the QR code and tap the Shepherdr banner. Confirm **Connect**.
- Or, in the app, tap **Scan QR code**, or type the hub's address (`hub.your-tailnet.ts.net`).

You should see your agents grouped as **Needs you / Error / Done / Working / Idle**.

## 6. Push notifications (optional)

1. **Create a key.** developer.apple.com → Certificates, IDs & Profiles → **Keys** → **+** → tick **Apple Push Notifications service (APNs)**. Download the `.p8` (you can only download it once) and note the **Key ID**.
2. **Put the key on the hub,** in the bridge's config folder:

   | Hub OS | Folder |
   |---|---|
   | macOS | `~/Library/Application Support/herdr-bridge/` |
   | Windows | `%AppData%\herdr-bridge\` |
   | Linux | `~/.config/herdr-bridge/` |

   Copy in two files:
   - `apns.p8`: the key you downloaded, renamed.
   - `apns.json`:
     ```json
     { "key_id": "ABC123DEFG", "team_id": "YOURTEAMID", "topic": "com.yourname.Shepherdr" }
     ```
     `topic` must be exactly your app's bundle ID.
3. **Restart the bridge:** run `install` again, or restart the service. `/v1/health` should now say `"push_enabled": true`.
4. **On the phone:** allow notifications when asked, then Settings (gear) → **Send test notification**.

Apps installed from Xcode use Apple's development push service. The app tells the bridge which one to use when it registers, so you don't need to configure anything. Keep the `.p8` out of git.

## 7. Using it

- **Agents list:** pull to refresh. The terminal icon shows plain shells too. **+** starts a new agent.
- **An agent:**
  - Its screen, refreshing while it works.
  - The message box sends a prompt.
  - The key bar answers dialogs (esc, ⏎, ↑/↓, 1/2/3, y/n, ^C).
  - **Stop** interrupts the agent.
  - The **terminal** icon opens the live terminal.
- **Live terminal:** full color and full-screen agent interfaces, with typing. While you type from the phone, the terminal is sized to the phone. Herdr on the desktop reflows until someone types there. Use ⋯ → **Watch only** to just look.
- **New agent:** pick the machine, the Herdr workspace and tab (or new ones), the folder, and Claude Code / Codex / Shell / a custom command, plus an optional first task. It appears in the Herdr desktop app too.
- **Notifications:**
  - You get one when an agent becomes **blocked**, **done** or hits an **error**, at most once per agent and state every 20 seconds.
  - **Approve** presses `1` for Claude Code, `y` for Codex and Enter for anything else. **Deny** presses Esc. Both need the phone unlocked, and both check first that the agent is still waiting.

## 8. Security model

- **Network:** the bridge listens on `127.0.0.1` only. Tailscale Serve is the only way in, so only devices on your tailnet can reach it.
- **Identity:** every request must carry your Tailscale identity (`Tailscale-User-Login`, set by Tailscale Serve and not forgeable through it). The owner is the hub's Tailscale user, and other users get `403`.
- **What the phone can do:** read terminals, type into them, interrupt, start terminals, label them, and upload files beside them. It **cannot**:
  - stop or restart the core
  - kill or remove terminals
  - run commands outside a terminal
  - read or write files
  - add or remove machines

  The full list is `bridge/internal/core/allow.go`.
- **Audit log:** every input is logged as `msg=input` in the bridge log, with the keys sent or the length of the text (never the text itself).

## 9. Troubleshooting

| Symptom | Fix |
|---|---|
| `install` stops at `tailscale serve` | Serve isn't enabled. Open the printed link, enable it, and run `install` again. |
| Browser or app can't connect right after setup | Wait about 10 s for the first certificate. Check that Tailscale is on, on the phone. |
| `403 forbidden` | The phone is signed into a different Tailscale user than the hub, or the hub is a tagged node (no user). Pass `-owner you@example.com` to `serve` if needed. |
| `/v1/health` shows `core_error` | Herdr isn't running on the hub. Start the Herdr app or `hc server start`. |
| Agents from another machine missing | The hub's core hasn't added that machine (`hc machine ls`), or it's asleep. |
| "<machine> is unavailable" / "needs login" | The hub's ssh to that machine wanted an interactive login. With Tailscale SSH this usually means the rule uses `"action": "check"`, which re-asks every ~12 h. Switch that rule to `"accept"`, or use regular sshd with a key. The bridge retries such machines every 1–15 minutes, and the app has a **Reconnect** button. |
| No notifications | Check `"push_enabled"` in `/v1/health`, then Settings → **Send test notification**, which reports Apple's error. `DeviceTokenNotForTopic` means `topic` ≠ the bundle ID. |
| Windows bridge not running after reboot | It starts at **logon**, so someone has to sign in to Windows. The log is `%LOCALAPPDATA%\Programs\herdr-bridge\herdr-bridge.log`. |
| Can't replace the bridge on Windows | A running `.exe` can't be overwritten. Always install from `herdr-bridge.new.exe`; the deploy script does this for you. |

## Uninstall

- **macOS:** `launchctl bootout gui/$(id -u)/dev.herdr-ui.bridge; rm ~/Library/LaunchAgents/dev.herdr-ui.bridge.plist`
- **Windows:**
  ```powershell
  reg delete HKCU\Software\Microsoft\Windows\CurrentVersion\Run /v herdr-bridge /f
  taskkill /IM herdr-bridge.exe /F
  schtasks /Delete /TN herdr-bridge /F
  ```
- **Everywhere:** `tailscale serve --https=8443 off`, and delete the config folder from step 6.
