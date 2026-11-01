# App Store Connect: what goes where

Everything below is ready to paste. Character counts are checked against Apple's limits.
Screenshots: `appstore/screenshots/` (6.9" iPhone, 1320×2868, no transparency). Upload them in this order:
1. `01-agents.png`
2. `02-notifications.png`
3. `03-blocked.png`
4. `04-live-terminal.png`
5. `05-new-agent.png`

The app icon comes from the build, so there's nothing to upload.

---

## App Information (App Store Connect → your app → App Information)

| Field | Value |
|---|---|
| **Name** (9/30) | Shepherdr |
| **Subtitle** (26/30) | Herd your AI coding agents |
| **Bundle ID** | app.shepherdr |
| **Primary category** | Developer Tools |
| **Secondary category** | Productivity |
| **Content rights** | Does not contain, show or access third-party content |
| **Age rating** | Answer **None / No** to every question → **4+** (links open in Safari; there's no in-app browser) |
| **Privacy Policy URL** | https://github.com/Richie97/shepherdr/blob/main/docs/PRIVACY.md |

## App Privacy (App Store Connect → App Privacy)

- **Do you or your third-party partners collect data from this app?** → **No, we do not collect data from this app.**
  The app talks only to the user's own hub. Nothing reaches the developer or any third party, so under Apple's definitions nothing is "collected".

## Version page (App Store → iOS App → 0.1.0)

**Promotional Text** (164/170)
```
Your coding agents keep working while you're away. Shepherdr tells you the moment one needs you, so you can approve it, steer it or start a new one from your phone.
```

**Description** (1834/4000)
```
Shepherdr keeps an eye on your AI coding agents so you don't have to sit at your desk.

It connects to Herdr, the terminal workspace for coding agents, on all of your machines at once, over your own private Tailscale network. See which agents are working, which are done, and which are waiting on you, then act on it from wherever you are.

NEVER MISS A BLOCKED AGENT
• Get a notification the moment an agent asks for permission, finishes, or hits an error
• Approve or deny right from the notification, without opening the app
• Blocked agents jump to the top, so nothing sits waiting

EVERY AGENT, EVERY MACHINE
• Agents from your laptop, desktop and servers in one list, grouped by what needs you
• See each agent's status, workspace and folder at a glance
• Machines that go offline are flagged, with one tap to reconnect

STEER FROM ANYWHERE
• Read an agent's screen and send it a prompt
• Quick keys for answering prompts: Esc, Enter, arrows, 1/2/3, y/n, Ctrl-C
• Stop an agent that's heading the wrong way

A REAL TERMINAL IN YOUR POCKET
• Attach to any agent's live terminal, in full color, with a keyboard
• Works with full-screen agent interfaces
• Watch-only mode when you just want to look

START AGENTS ON THE GO
• Pick a machine, a workspace, a folder and an agent, then give it a first task
• New agents appear in Herdr on your desktop too

PRIVATE BY DESIGN
• Talks only to a small open-source bridge you run on one of your machines
• Reached over your Tailscale network; nothing is exposed to the internet
• No accounts, no analytics, no data collected

REQUIREMENTS
Shepherdr is a companion app. You'll need:
• Herdr running on your machines
• Tailscale on your computers and iPhone
• herdr-bridge running on one machine (free and open source; setup takes about 15 minutes)

Setup guide: github.com/Richie97/shepherdr
```

**Keywords** (96/100). Words from the name and subtitle are indexed already, so they aren't repeated. No third-party trademarks, per App Review guideline 2.3.7.
```
terminal,developer,remote,ssh,cli,llm,devops,monitor,approve,notify,shell,automation,programming
```

**Support URL:** https://github.com/Richie97/shepherdr/issues
**Marketing URL:** https://github.com/Richie97/shepherdr
**Copyright:** 2026 Eric Richardson

**What's New** (155/4000)
```
First release: agents across all your machines, push notifications with Approve and Deny, a live terminal, starting agents from your phone, and QR pairing.
```

## App Review Information

- **Sign-in required:** No. There are no accounts.
- **Notes:**
```
Shepherdr is a companion app for Herdr (herdr.dev), a terminal workspace for AI coding agents. It connects only to a bridge the user runs on their own computer, reached over the user's private Tailscale network, so it can't connect to anything without that setup. There is no account or login.

What reviewers will see without a hub: the Connect screen, which asks for the hub's address or a QR code.

The bridge is free and open source: https://github.com/Richie97/shepherdr (setup guide in docs/SETUP.md). A screen recording of the app connected to a hub is attached.

The app collects no data. Push tokens are sent only to the user's own hub. The camera is used only to scan the pairing QR code. Encryption: HTTPS/TLS only (exempt).
```
- **Attachment:** a 30–60 s screen recording of the app connected to your hub, showing the agents list, a notification with Approve, the live terminal and New agent. Reviewers can't reach your tailnet, so without the recording an App Store submission is likely to be rejected under 2.1 (App Completeness).

## Export compliance

Already answered in the build: `ITSAppUsesNonExemptEncryption = NO`, since the app uses only HTTPS/TLS. App Store Connect won't ask.

---

## TestFlight (App Store Connect → TestFlight)

**Beta App Description** (250/4000)
```
Shepherdr is a mobile companion for Herdr: monitor and control AI coding agents on all your machines over Tailscale. Get notified when an agent needs you, approve from the notification, attach to a live terminal, and start new agents from your phone.
```

**What to Test** (495/4000), per build:
```
Pair with your hub (scan the QR code from herdr-bridge pair or https://<hub>:8443/pair), then try:
• Agents list: do states (Needs you, Working, Done, Idle) match Herdr on your desktop?
• Notifications: does a blocked agent alert you, and do Approve and Deny do the right thing?
• Live terminal: open an agent, tap the terminal icon, type a little, and try Watch only.
• New agent: start one with a short task and check that it shows up in Herdr.
Send feedback with a screenshot from TestFlight.
```

- **Feedback email:** your address.
- **Internal testing:** add yourself (and any team members) to an internal group. No review, and builds appear right after processing.
- **External testing:** needs Beta App Review. Paste the App Review notes above and attach the screen recording.
