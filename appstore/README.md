# App Store assets

- `metadata.md`: every App Store Connect and TestFlight text field, ready to paste, with character counts.
- `screenshots/`: 6.9" iPhone screenshots (1320×2868) with captions.
- `make-screenshots.swift`: frames raw simulator screenshots into those images.
- Privacy policy: [`docs/PRIVACY.md`](../docs/PRIVACY.md), linked from the listing.

## Regenerating the screenshots

The raw screenshots come from the app's **demo mode**: Debug builds only, made-up agents, no hub needed.

```sh
cd ios && xcodegen generate
SIM=<an iPhone 17 Pro Max simulator id>   # xcrun simctl list devices available
xcodebuild -scheme Shepherdr -destination "id=$SIM" -derivedDataPath build -skipPackagePluginValidation build
xcrun simctl boot $SIM; xcrun simctl ui $SIM appearance dark
xcrun simctl status_bar $SIM override --time 9:41 --batteryState charged --batteryLevel 100 --wifiBars 3 --cellularBars 4
xcrun simctl install $SIM build/Build/Products/Debug-iphonesimulator/Shepherdr.app
RAW=/tmp/shepherdr-raw; mkdir -p $RAW
shot() { xcrun simctl terminate $SIM app.shepherdr; xcrun simctl launch $SIM app.shepherdr -demo YES "${@:2}"; sleep 6; xcrun simctl io $SIM screenshot $RAW/$1.png; }
shot 1-agents
shot 2-blocked -openTerminal laptop/t4
shot 3-live -openTerminal laptop/t4 -openLive YES
shot 4-new -openNewAgent YES
cd .. && swift appstore/make-screenshots.swift $RAW ios/Shepherdr/Assets.xcassets/AppIcon.appiconset/AppIcon.png appstore/screenshots
```

Captions and colors are at the bottom of `make-screenshots.swift`. The demo data is in `ios/Shepherdr/Views/DemoData.swift`.
