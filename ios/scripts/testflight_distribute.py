#!/usr/bin/env python3
"""Talk to App Store Connect for TestFlight:

  --check          verify the API key can see the app (preflight)
  --upload IPA     upload a signed .ipa through the Build Uploads API
  --build N        wait for build N to process, set What to Test, add it to a group

xcodebuild's own upload (-exportArchive with destination=upload) rejects
team API keys with "Cannot verify client's JWT headers" (CDWebService 3004)
even when the key works everywhere else, so CI exports the .ipa locally and
uploads it here instead.

Standard library only; the API token is signed with the `openssl` CLI.

  ASC_KEY_PATH=... ASC_KEY_ID=... ASC_ISSUER_ID=... \\
  ios/scripts/testflight_distribute.py --build 202611011900 \\
      [--bundle-id app.shepherdr] [--group Internal] [--notes-file notes.txt] [--timeout 1800]

A group with automatic distribution ("access to all builds") already gets
every build, so adding is skipped for it; notes are still set.
"""
import argparse
import base64
import hashlib
import json
import os
import subprocess
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

API = "https://api.appstoreconnect.apple.com"


def b64url(b: bytes) -> str:
    return base64.urlsafe_b64encode(b).rstrip(b"=").decode()


def der_to_raw(der: bytes) -> bytes:
    """ECDSA DER signature (SEQUENCE{INTEGER r, INTEGER s}) -> 64-byte r||s for JWS."""
    def read_len(b, i):
        n = b[i]
        if n < 0x80:
            return n, i + 1
        k = n & 0x7F
        return int.from_bytes(b[i + 1:i + 1 + k], "big"), i + 1 + k

    assert der[0] == 0x30, "not a DER sequence"
    _, i = read_len(der, 1)
    out = b""
    for _ in range(2):
        assert der[i] == 0x02, "expected INTEGER"
        n, i = read_len(der, i + 1)
        v = der[i:i + n].lstrip(b"\x00")
        out += v.rjust(32, b"\x00")
        i += n
    return out


def token(key_path: str, key_id: str, issuer: str, now: int | None = None) -> str:
    now = int(now or time.time())
    header = {"alg": "ES256", "kid": key_id, "typ": "JWT"}
    claims = {"iss": issuer, "iat": now, "exp": now + 15 * 60, "aud": "appstoreconnect-v1"}
    signing_input = f"{b64url(json.dumps(header).encode())}.{b64url(json.dumps(claims).encode())}"
    der = subprocess.run(["openssl", "dgst", "-sha256", "-sign", key_path], input=signing_input.encode(),
                         capture_output=True, check=True).stdout
    return f"{signing_input}.{b64url(der_to_raw(der))}"


class ASC:
    def __init__(self, key_path, key_id, issuer):
        self.key = (key_path, key_id, issuer)
        self._tok, self._at = None, 0

    def call(self, method, path, body=None, ok_codes=(200, 201, 204)):
        """JSON request to the App Store Connect API."""
        if time.time() - self._at > 600:
            self._tok, self._at = token(*self.key), time.time()
        req = urllib.request.Request(API + path, method=method,
                                     data=json.dumps(body).encode() if body is not None else None,
                                     headers={"Authorization": f"Bearer {self._tok}", "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                raw = r.read()
                return json.loads(raw) if raw else {}
        except urllib.error.HTTPError as e:
            if e.code in ok_codes:
                return {}
            detail = e.read().decode(errors="replace")[:800]
            raise SystemExit(f"App Store Connect {method} {path}: HTTP {e.code}\n{detail}")


def put_part(op: dict, data: bytes):
    """Send one byte range to the URL App Store Connect handed out (no API auth)."""
    headers = {h["name"]: h["value"] for h in op.get("requestHeaders") or []}
    req = urllib.request.Request(op["url"], data=data, method=op.get("method", "PUT"), headers=headers)
    for attempt in range(4):
        try:
            with urllib.request.urlopen(req, timeout=300) as r:
                r.read()
                return
        except (urllib.error.URLError, TimeoutError) as e:
            if attempt == 3:
                raise SystemExit(f"upload of part {op.get('partNumber')} failed: {e}")
            time.sleep(2 ** attempt)


def upload_ipa(asc, app_id: str, ipa: str, version: str, build: str, timeout: int):
    size = os.path.getsize(ipa)
    md5 = hashlib.md5()
    with open(ipa, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            md5.update(chunk)

    up = asc.call("POST", "/v1/buildUploads", {"data": {
        "type": "buildUploads",
        "attributes": {"cfBundleShortVersionString": version, "cfBundleVersion": build, "platform": "IOS"},
        "relationships": {"app": {"data": {"type": "apps", "id": app_id}}}}})["data"]
    print(f"build upload {up['id']} created for {version} ({build})", flush=True)

    file = asc.call("POST", "/v1/buildUploadFiles", {"data": {
        "type": "buildUploadFiles",
        "attributes": {"assetType": "ASSET", "fileName": os.path.basename(ipa), "fileSize": size, "uti": "com.apple.ipa"},
        "relationships": {"buildUpload": {"data": {"type": "buildUploads", "id": up["id"]}}}}})["data"]
    ops = file["attributes"].get("uploadOperations") or []
    print(f"uploading {size / 1e6:.1f} MB in {len(ops)} part(s)", flush=True)
    with open(ipa, "rb") as f:
        for op in sorted(ops, key=lambda o: o.get("offset", 0)):
            f.seek(op.get("offset", 0))
            put_part(op, f.read(op.get("length", size)))

    asc.call("PATCH", f"/v1/buildUploadFiles/{file['id']}", {"data": {
        "type": "buildUploadFiles", "id": file["id"],
        "attributes": {"uploaded": True, "sourceFileChecksums": {"file": {"algorithm": "MD5", "hash": md5.hexdigest()}}}}})
    print("upload committed; waiting for App Store Connect to accept it", flush=True)

    deadline = time.time() + timeout
    while time.time() < deadline:
        st = asc.call("GET", f"/v1/buildUploads/{up['id']}")["data"]["attributes"].get("state") or {}
        state = st.get("state")
        for kind in ("errors", "warnings"):
            for d in st.get(kind) or []:
                print(f"  {kind[:-1]}: {d.get('code')}: {d.get('description')}", flush=True)
        print(f"upload state: {state}", flush=True)
        if state == "COMPLETE":
            return
        if state == "FAILED":
            raise SystemExit("App Store Connect rejected the upload (see errors above)")
        time.sleep(20)
    raise SystemExit(f"upload not accepted within {timeout}s")


def q(params: dict) -> str:
    return "?" + urllib.parse.urlencode(params)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--build", help="CFBundleVersion of the uploaded build")
    ap.add_argument("--upload", metavar="IPA", help="upload this signed .ipa (needs --version and --build)")
    ap.add_argument("--version", help="CFBundleShortVersionString, for --upload")
    ap.add_argument("--check", action="store_true",
                    help="only verify the API key can see the app (preflight before archiving)")
    ap.add_argument("--bundle-id", default="app.shepherdr")
    ap.add_argument("--group", default="Internal")
    ap.add_argument("--notes-file", help="What to Test text (max 4000 chars)")
    ap.add_argument("--timeout", type=int, default=1800, help="seconds to wait for processing")
    a = ap.parse_args()
    asc = ASC(os.environ["ASC_KEY_PATH"], os.environ["ASC_KEY_ID"], os.environ["ASC_ISSUER_ID"])

    apps = asc.call("GET", "/v1/apps" + q({"filter[bundleId]": a.bundle_id, "fields[apps]": "bundleId,name"}))["data"]
    if not apps:
        raise SystemExit(f"No App Store Connect app with bundle ID {a.bundle_id}")
    app_id = apps[0]["id"]
    if a.check:
        groups = asc.call("GET", f"/v1/apps/{app_id}/betaGroups" + q({"fields[betaGroups]": "name", "limit": 200}))["data"]
        print(f'API key OK: app "{apps[0]["attributes"]["name"]}" ({a.bundle_id}), TestFlight groups: '
              + (", ".join(g["attributes"]["name"] for g in groups) or "none"))
        return
    if not a.build:
        ap.error("--build is required")
    if a.upload:
        if not a.version:
            ap.error("--upload needs --version")
        upload_ipa(asc, app_id, a.upload, a.version, a.build, a.timeout)
        return

    # The build shows up a few minutes after upload, then processes.
    deadline = time.time() + a.timeout
    build = None
    while time.time() < deadline:
        found = asc.call("GET", "/v1/builds" + q({"filter[app]": app_id, "filter[version]": a.build,
                                                  "fields[builds]": "version,processingState"}))["data"]
        state = found[0]["attributes"]["processingState"] if found else "NOT_YET_VISIBLE"
        print(f"build {a.build}: {state}", flush=True)
        if state == "VALID":
            build = found[0]
            break
        if state in ("FAILED", "INVALID"):
            raise SystemExit(f"Build {a.build} processing {state}; see App Store Connect for details")
        time.sleep(30)
    if not build:
        raise SystemExit(f"Build {a.build} wasn't processed within {a.timeout}s")
    build_id = build["id"]

    if a.notes_file:
        notes = open(a.notes_file).read().strip()[:4000]
        locs = asc.call("GET", f"/v1/builds/{build_id}/betaBuildLocalizations")["data"]
        if locs:
            asc.call("PATCH", f"/v1/betaBuildLocalizations/{locs[0]['id']}",
                     {"data": {"type": "betaBuildLocalizations", "id": locs[0]["id"], "attributes": {"whatsNew": notes}}})
        else:
            asc.call("POST", "/v1/betaBuildLocalizations",
                     {"data": {"type": "betaBuildLocalizations", "attributes": {"locale": "en-US", "whatsNew": notes},
                               "relationships": {"build": {"data": {"type": "builds", "id": build_id}}}}})
        print("set What to Test")

    groups = asc.call("GET", f"/v1/apps/{app_id}/betaGroups" + q({"fields[betaGroups]": "name,isInternalGroup,hasAccessToAllBuilds", "limit": 200}))["data"]
    group = next((g for g in groups if g["attributes"]["name"] == a.group), None)
    if not group:
        names = ", ".join(g["attributes"]["name"] for g in groups) or "none"
        raise SystemExit(f'No TestFlight group named "{a.group}" (groups: {names})')
    if group["attributes"].get("hasAccessToAllBuilds"):
        print(f'"{a.group}" has automatic distribution; build {a.build} is already available to it')
        return
    asc.call("POST", f"/v1/betaGroups/{group['id']}/relationships/builds",
             {"data": [{"type": "builds", "id": build_id}]}, ok_codes=(204, 409))
    print(f'added build {a.build} to "{a.group}"')


if __name__ == "__main__":
    main()
