# Shepherdr Privacy Policy

_Last updated: November 1, 2026_

Shepherdr is an iPhone app for monitoring and controlling coding agents running in [Herdr](https://herdr.dev) on your own computers.

## The short version

Shepherdr does not collect, store or share any of your data. There are no accounts, analytics, ads or trackers. The developer has no servers that your data passes through.

## How the app works

- Shepherdr connects only to a hub you run yourself: the open-source `herdr-bridge` on one of your machines, reached over your private Tailscale network.
- Everything the app shows comes straight from your hub: terminal output, agent names, folders and statuses. Everything you send goes straight to it: prompts, keystrokes and files.
- The developer can't see any of it.
- The app stores only your hub's address and a few display preferences, on your phone.

## Push notifications

If you allow notifications, your phone's push token is sent to **your own hub**, which uses it to send alerts through Apple Push Notification service when an agent needs you. Notification text (for example, an agent's name and machine) travels from your hub through Apple's service to your phone. The token is kept on your hub only, and you can remove it by disconnecting the app.

## Camera

The camera is used only when you choose **Scan QR code**, to read your hub's pairing code. Images are processed on the device and are never stored or sent anywhere.

## Third parties

Shepherdr includes no third-party analytics or advertising code. Network traffic goes over Tailscale (governed by [Tailscale's privacy policy](https://tailscale.com/privacy-policy)) to your own machines, and notifications go through Apple's push service.

## Children

Shepherdr is a developer tool and isn't directed at children.

## Changes and contact

Changes to this policy are published at this address, and the history is in the project's repository. For questions, open an issue at <https://github.com/Richie97/shepherdr/issues>.
