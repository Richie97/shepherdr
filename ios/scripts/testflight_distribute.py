#!/usr/bin/env python3
"""After an upload: wait for App Store Connect to process the build, add it to a
TestFlight group, and set its "What to Test" notes.

Standard library only; the API token is signed with the `openssl` CLI.

  ASC_KEY_PATH=... ASC_KEY_ID=... ASC_ISSUER_ID=... \\
  ios/scripts/testflight_distribute.py --build 202611011900 \\
      [--bundle-id app.shepherdr] [--group Internal] [--notes-file notes.txt] [--timeout 1800]

A group with automatic distribution ("access to all builds") already gets
every build, so adding is skipped for it; notes are still set.
"""
import argparse
import base64
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


def q(params: dict) -> str:
    return "?" + urllib.parse.urlencode(params)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--build", required=True, help="CFBundleVersion of the uploaded build")
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
