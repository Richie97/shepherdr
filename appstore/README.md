# App Store assets

- `metadata/`: the listing text, one file per field. Pushed to App Store Connect automatically when it changes on main.
- `metadata.md`: which file is which field, plus the parts that stay manual (age rating, privacy answers, review notes).
- `screenshots/`: 6.9" iPhone screenshots (1320×2868) with captions. Also synced; their order follows the file names.
- `make-screenshots.swift`: frames raw simulator screenshots into those images.
- Privacy policy: the listing links to https://speedshift.io/privacy (the Shepherdr section of the Speedshift policy). [`docs/PRIVACY.md`](../docs/PRIVACY.md) is the same text; keep both in sync.

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

## Syncing to App Store Connect

`ios/scripts/appstore_sync.py` pushes `metadata/` and `screenshots/` to App Store Connect. It runs in the TestFlight workflow when these change on main, or on a manual run. To run it locally, use the same API key as TestFlight:

```sh
python3 ios/scripts/appstore_sync.py --check          # offline: lengths, screenshot sizes
ASC_KEY_PATH=AuthKey.p8 ASC_KEY_ID=... ASC_ISSUER_ID=... python3 ios/scripts/appstore_sync.py
```

It never submits for review. A version that's in review or live can't be edited, so it's skipped; bump `MARKETING_VERSION` to start the next one.
