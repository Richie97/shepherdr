# App Store assets

- `metadata/`: the listing text, one file per field. Pushed to App Store Connect automatically when it changes on main.
- `metadata.md`: which file is which field, plus the parts that stay manual (age rating, privacy answers, review notes).
- `screenshots/`: 6.9" iPhone screenshots (1320×2868) with captions. Also synced; their order follows the file names.
- `screenshots-ipad/`: 13" iPad screenshots (2048×2732), the same five frames. Synced the same way.
- `make-screenshots.swift`: frames raw simulator screenshots into those images (`ipad` as a 4th argument for the iPad set).
- `creative/`: the iOS 27 product page header (`header.png`, 3840×1646) and search results image (`search.png`, 3840×2560). Not synced: upload them by hand (see `metadata.md`).
- `make-creative.swift`: draws those two from the same raw screenshots, with the app icon's sheep redrawn as vectors.
- App icon: comes from the build, `ios/Shepherdr/Assets.xcassets/AppIcon.appiconset/AppIcon.png` (1024×1024, no alpha).
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
swift appstore/make-creative.swift $RAW appstore/creative   # header and search results images
```

For the iPad set, run the same steps on an **iPad Air 13-inch** simulator (its screenshots are 2048×2732), with two changes: `shot 1-agents -openTerminal t2` (so the detail pane isn't empty), and frame with `ipad` as a 4th argument into `appstore/screenshots-ipad`:

```sh
swift appstore/make-screenshots.swift $RAW ios/Shepherdr/Assets.xcassets/AppIcon.appiconset/AppIcon.png appstore/screenshots-ipad ipad
```

The header keeps its text inside Apple's safe area (x 1097–2743, y 493–1154), and the search image keeps the phones inside its own (x 836–3004, y 765–1795). Everything outside those areas is scenery that may be cropped.

Captions and colors are at the bottom of `make-screenshots.swift`. The demo data is in `ios/Shepherdr/Views/DemoData.swift`.

## Syncing to App Store Connect

`ios/scripts/appstore_sync.py` pushes `metadata/`, `screenshots/` and `screenshots-ipad/` to App Store Connect. It runs in the TestFlight workflow when these change on main, or on a manual run. To run it locally, use the same API key as TestFlight:

```sh
python3 ios/scripts/appstore_sync.py --check          # offline: lengths, screenshot sizes
ASC_KEY_PATH=AuthKey.p8 ASC_KEY_ID=... ASC_ISSUER_ID=... python3 ios/scripts/appstore_sync.py
```

It never submits for review. A version that's in review or live can't be edited, so it's skipped; bump `MARKETING_VERSION` to start the next one.
