#!/usr/bin/env bash
# Archive Shepherdr and upload it to App Store Connect for TestFlight.
#
#   ios/scripts/testflight.sh              # archive + upload
#   ios/scripts/testflight.sh --no-upload  # archive + export locally only
#
# Signs automatically with the team in project.yml. Authentication is either
# the Xcode account signed in on this Mac, or an App Store Connect API key in
# ASC_KEY_PATH / ASC_KEY_ID / ASC_ISSUER_ID (what CI uses). The build number
# is a UTC timestamp so every upload is unique; bump MARKETING_VERSION in
# project.yml for a new user-facing version.
#
# Full logs: ios/build/testflight/{archive,export}.log
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
  : "${ASC_KEY_ID:?ASC_KEY_ID is required with ASC_KEY_PATH}" "${ASC_ISSUER_ID:?ASC_ISSUER_ID is required with ASC_KEY_PATH}"
  auth=(-authenticationKeyPath "$ASC_KEY_PATH" -authenticationKeyID "$ASC_KEY_ID" -authenticationKeyIssuerID "$ASC_ISSUER_ID")
fi

# Runs xcodebuild with the log in a file, then shows the lines that matter.
run() {
  local log=$1; shift
  if xcodebuild "$@" > "$log" 2>&1; then
    grep -E 'warning: .*Shepherdr/|\*\* (ARCHIVE|EXPORT) SUCCEEDED|Upload succeeded|Uploaded' "$log" || true
  else
    grep -E 'error|\*\* (ARCHIVE|EXPORT) FAILED' "$log" | head -40 >&2 || tail -40 "$log" >&2
    echo "xcodebuild failed; full log: ios/$log" >&2
    exit 1
  fi
}

xcodegen generate -q
echo "archiving build ${build}..."
run "$out/archive.log" archive -scheme Shepherdr -configuration Release \
  -destination 'generic/platform=iOS' -archivePath "$out/Shepherdr.xcarchive" \
  -derivedDataPath build -allowProvisioningUpdates -skipPackagePluginValidation \
  CURRENT_PROJECT_VERSION="$build" ${auth[@]+"${auth[@]}"}

opts=ExportOptions.plist
if [[ $upload == 0 ]]; then
  opts="$out/ExportOptions-local.plist"
  sed 's#<string>upload</string>#<string>export</string>#' ExportOptions.plist > "$opts"
fi
echo "exporting$([[ $upload == 1 ]] && echo ' and uploading')..."
run "$out/export.log" -exportArchive -archivePath "$out/Shepherdr.xcarchive" -exportOptionsPlist "$opts" \
  -exportPath "$out/export" -allowProvisioningUpdates ${auth[@]+"${auth[@]}"}

version=$(/usr/libexec/PlistBuddy -c 'Print :ApplicationProperties:CFBundleShortVersionString' "$out/Shepherdr.xcarchive/Info.plist")
if [[ $upload == 1 ]]; then
  echo "Uploaded Shepherdr $version ($build). It appears in TestFlight after Apple finishes processing (usually 5-15 min)."
else
  echo "Exported Shepherdr $version ($build) to ios/$out/export"
fi
