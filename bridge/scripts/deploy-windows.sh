#!/usr/bin/env bash
# Build herdr-bridge for Windows and install it on a machine your Herdr core
# has added (`hc machine ls`), using only Herdr's own link: no ssh/scp needed.
#
#   bridge/scripts/deploy-windows.sh <machine>
#
# Uploads in chunks (hc file write takes at most 8 MB), joins them, checks
# the hash, then runs `herdr-bridge.new.exe install`, which stops a running
# bridge, copies itself into place, and starts it.
set -euo pipefail
machine=${1:?usage: deploy-windows.sh <machine>}
hc=$(command -v hc || command -v herdr-core) || { echo "hc not found" >&2; exit 1; }
here=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

echo "building…"
(cd "$here" && GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H=windowsgui" -o "$work/herdr-bridge.exe" ./cmd/herdr-bridge)

b64() { python3 -I -c 'import json,sys,base64; d=json.load(sys.stdin); sys.stdout.write(base64.b64decode(d.get("stdout","")).decode())'; }
local_app=$("$hc" exec --on "$machine" --json cmd /c "echo %LOCALAPPDATA%" | b64 | tr -d '\r\n')
dir="$local_app\\Programs\\herdr-bridge"
"$hc" exec --on "$machine" --json cmd /c "if not exist \"$dir\" mkdir \"$dir\"" >/dev/null

echo "uploading to $machine:$dir…"
(cd "$work" && split -b 6000000 herdr-bridge.exe part.)
for p in "$work"/part.*; do
  "$hc" file write --on "$machine" "$dir\\$(basename "$p")" < "$p" >/dev/null
done
remote=$("$hc" exec --on "$machine" --json powershell -NoProfile -Command \
  "\$d='$dir'; \$o=[IO.File]::Create(\"\$d\\herdr-bridge.new.exe\"); foreach(\$p in (Get-ChildItem \"\$d\\part.*\" | Sort-Object Name)){ \$b=[IO.File]::ReadAllBytes(\$p.FullName); \$o.Write(\$b,0,\$b.Length); Remove-Item \$p.FullName }; \$o.Close(); (Get-FileHash \"\$d\\herdr-bridge.new.exe\" -Algorithm SHA256).Hash" \
  | b64 | tr -d '\r\n' | tr 'A-F' 'a-f')
want=$(shasum -a 256 "$work/herdr-bridge.exe" | cut -d' ' -f1)
[[ "$remote" == "$want" ]] || { echo "hash mismatch: $remote != $want" >&2; exit 1; }

echo "installing…"
"$hc" exec --on "$machine" --json "$dir\\herdr-bridge.new.exe" install | b64
