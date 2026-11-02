# App Store Connect: what goes where

**Automated:** the listing below is pushed to App Store Connect by the `Sync App Store listing` job in `.github/workflows/testflight.yml`, whenever `appstore/` changes on main (`ios/scripts/appstore_sync.py`). Edit the files, not App Store Connect, or the next sync overwrites your edits. CI checks character limits and screenshot sizes on every pull request (`appstore_sync.py --check`).

| Field | File |
|---|---|
| Name (30), Subtitle (30) | `metadata/en-US/name.txt`, `subtitle.txt` |
| Promotional Text (170) | `metadata/en-US/promotional_text.txt` |
| Description (4000) | `metadata/en-US/description.txt` |
| Keywords (100) | `metadata/en-US/keywords.txt` |
| What's New (4000) | `metadata/en-US/release_notes.txt` (skipped for the first release; Apple doesn't allow it) |
| Support, Marketing, Privacy Policy URLs | `metadata/en-US/support_url.txt`, `marketing_url.txt`, `privacy_url.txt` |
| TestFlight Beta App Description | `metadata/en-US/beta_description.txt` |
| Copyright | `metadata/copyright.txt` |
| Categories | `metadata/primary_category.txt`, `secondary_category.txt` (App Store Connect IDs, like `DEVELOPER_TOOLS`) |
| Screenshots (6.9") | `screenshots/*.png`, uploaded in file-name order, replaced only when they change |
| Version | `MARKETING_VERSION` in `ios/project.yml`. The sync creates that version, or renames the one being prepared |

Notes on the text: words from the name and subtitle are indexed already, so keywords don't repeat them. No third-party trademarks in keywords (App Review guideline 2.3.7). The app icon comes from the build.

**Still manual** (once, or per release): everything below, plus **Submit for Review**.

---

## App Information (manual)

| Field | Value |
|---|---|
| **Content rights** | Does not contain, show or access third-party content |
| **Age rating** | Answer **None / No** to every question → **4+** (links open in Safari; there's no in-app browser) |

## App Privacy (App Store Connect → App Privacy)

- **Do you or your third-party partners collect data from this app?** → **No, we do not collect data from this app.**
  The app talks only to the user's own hub. Nothing reaches the developer or any third party, so under Apple's definitions nothing is "collected".

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

## Header and Search Results (iOS 27)

On the version's page: **Product Page Information → Header and Search Results**, upload:

| Placement | File | Spec |
|---|---|---|
| Product page header | `creative/header.png` | 3840×1646 (21:9), one image for every language |
| Search results | `creative/search.png` | 3840×2560 (3:2), set per language |

Both are PNG without alpha. They're reviewed with the version, so upload them before **Submit for Review**, and check them with **Preview**. They show only on iOS 27 and later. Older versions keep showing screenshots in Search.

## Export compliance

Already answered in the build: `ITSAppUsesNonExemptEncryption = NO`, since the app uses only HTTPS/TLS. App Store Connect won't ask.

---

## TestFlight (App Store Connect → TestFlight)

Beta App Description: synced from `metadata/en-US/beta_description.txt`.

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
