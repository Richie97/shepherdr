#!/usr/bin/env bash
# Archive Shepherdr and upload it to App Store Connect for TestFlight.
#
#   ios/scripts/testflight.sh            # archive + upload
#   ios/scripts/testflight.sh --no-upload  # archive + export locally only
#
# Signs with the Xcode account's team (automatic signing). The build number is
# a UTC timestamp so every upload is unique; bump MARKETING_VERSION in
# project.yml for a new user-facing version.
#
# To upload without an Xcode account signed in, set ASC_KEY_PATH, ASC_KEY_ID
# and ASC_ISSUER_ID (an App Store Connect API key with App Manager access).
set -euo pipefail
cd "$(dirname "$0")/.."
upload=1
[[ "${1:-}" == "--no-upload" ]] && upload=0

build=$(date -u +%Y%m%d%H%M)
out=build/testflight
rm -rf "$out"
mkdir -p "$out"

auth=()
if [[ -n "${ASC_KEY_PATH:-}" ]]; then
  auth=(-authenticationKeyPath "$ASC_KEY_PATH" -authenticationKeyID "$ASC_KEY_ID" -authenticationKeyIssuerID "$ASC_ISSUER_ID")
fi

xcodegen generate -q
echo "archiving build $build…"
xcodebuild archive -scheme Shepherdr -configuration Release \
  -destination 'generic/platform=iOS' -archivePath "$out/Shepherdr.xcarchive" \
  -derivedDataPath build -allowProvisioningUpdates -skipPackagePluginValidation \
  CURRENT_PROJECT_VERSION="$build" "${auth[@]}" | grep -E 'error:|warning: .*Shepherdr/|\*\* ARCHIVE' || true
[[ -d "$out/Shepherdr.xcarchive" ]] || { echo "archive failed" >&2; exit 1; }

opts=ExportOptions.plist
if [[ $upload == 0 ]]; then
  opts="$out/ExportOptions-local.plist"
  sed 's#<string>upload</string>#<string>export</string>#' ExportOptions.plist > "$opts"
fi
echo "exporting$([[ $upload == 1 ]] && echo ' and uploading')…"
xcodebuild -exportArchive -archivePath "$out/Shepherdr.xcarchive" -exportOptionsPlist "$opts" \
  -exportPath "$out/export" -allowProvisioningUpdates "${auth[@]}" 2>&1 | grep -E 'error|Upload|Export|\*\* EXPORT' || true

version=$(/usr/libexec/PlistBuddy -c 'Print :ApplicationProperties:CFBundleShortVersionString' "$out/Shepherdr.xcarchive/Info.plist")
echo "Shepherdr $version ($build)"
