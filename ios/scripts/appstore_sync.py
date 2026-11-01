#!/usr/bin/env python3
"""Push the App Store listing in appstore/ to App Store Connect.

  appstore/metadata/en-US/*.txt   listing text, one file per field
  appstore/metadata/*.txt         copyright and categories
  appstore/screenshots/*.png      6.9" iPhone screenshots, uploaded in name order

What it does, all idempotent:
  - App Information: name, subtitle, privacy policy URL, categories
  - the App Store version for MARKETING_VERSION (ios/project.yml): creates it,
    or renames the version that's being prepared; sets copyright
  - that version's text: description, keywords, promotional text, support and
    marketing URLs, and What's New (skipped for the first release, which
    Apple doesn't allow)
  - screenshots: replaced only when the files differ (by MD5), then ordered
  - TestFlight beta app description

It never submits for review. A version that's in review or live can't be
edited, so it's skipped with a notice.

  ASC_KEY_PATH=... ASC_KEY_ID=... ASC_ISSUER_ID=... ios/scripts/appstore_sync.py
  ios/scripts/appstore_sync.py --check   # validate the files offline (lengths, sizes)
"""
import argparse
import hashlib
import os
import re
import struct
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from testflight_distribute import ASC, put_part, q  # noqa: E402

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
META = os.path.join(ROOT, "appstore", "metadata")
SHOTS = os.path.join(ROOT, "appstore", "screenshots")
LOCALE = "en-US"
DISPLAY_TYPE = "APP_IPHONE_67"  # the 6.9" slot; 1320x2868 fits it
SHOT_SIZES = {(1320, 2868), (1290, 2796), (1260, 2736)}

LIMITS = {"name": 30, "subtitle": 30, "promotional_text": 170, "description": 4000,
          "keywords": 100, "release_notes": 4000, "beta_description": 4000}
EDITABLE = {"PREPARE_FOR_SUBMISSION", "DEVELOPER_REJECTED", "REJECTED", "METADATA_REJECTED", "INVALID_BINARY"}
RELEASED = {"READY_FOR_SALE", "READY_FOR_DISTRIBUTION", "REPLACED_WITH_NEW_VERSION",
            "REMOVED_FROM_SALE", "DEVELOPER_REMOVED_FROM_SALE"}


def read(*parts) -> str:
    with open(os.path.join(META, *parts)) as f:
        return f.read().strip()


def text() -> dict:
    return {os.path.splitext(f)[0]: read(LOCALE, f)
            for f in sorted(os.listdir(os.path.join(META, LOCALE))) if f.endswith(".txt")}


def marketing_version() -> str:
    m = re.search(r'MARKETING_VERSION:\s*"?([0-9][0-9.]*)', open(os.path.join(ROOT, "ios", "project.yml")).read())
    if not m:
        raise SystemExit("MARKETING_VERSION not found in ios/project.yml")
    return m.group(1)


def screenshots() -> list:
    return sorted(os.path.join(SHOTS, f) for f in os.listdir(SHOTS) if f.lower().endswith(".png"))


def png_info(path):
    with open(path, "rb") as f:
        head = f.read(26)
    if head[:8] != b"\x89PNG\r\n\x1a\n":
        raise SystemExit(f"{path}: not a PNG")
    w, h = struct.unpack(">II", head[16:24])
    return w, h, head[25]  # color type 6/4 = has alpha


def md5(path) -> str:
    h = hashlib.md5()
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            h.update(chunk)
    return h.hexdigest()


def check() -> dict:
    t = text()
    problems = []
    for k, limit in LIMITS.items():
        if k in t and len(t[k]) > limit:
            problems.append(f"{k}: {len(t[k])} characters (limit {limit})")
    if "keywords" in t and len(t["keywords"].encode()) > 100:
        problems.append("keywords: over 100 bytes")
    shots = screenshots()
    if not 1 <= len(shots) <= 10:
        problems.append(f"{len(shots)} screenshots (need 1-10)")
    for s in shots:
        w, h, color = png_info(s)
        if (w, h) not in SHOT_SIZES:
            problems.append(f"{os.path.basename(s)}: {w}x{h} isn't a 6.9\" size")
        if color in (4, 6):
            problems.append(f"{os.path.basename(s)}: has an alpha channel")
    for p in problems:
        print(f"problem: {p}")
    if problems:
        raise SystemExit("App Store files need fixing")
    print(f"listing OK: {len(t)} text fields, {len(shots)} screenshots, version {marketing_version()}")
    return t


def find_or_create(asc, list_path, create_body, match):
    for item in asc.call("GET", list_path)["data"]:
        if match(item):
            return item
    return asc.call("POST", create_body[0], create_body[1])["data"]


def patch(asc, kind, id_, attrs, rels=None):
    body = {"data": {"type": kind, "id": id_, "attributes": attrs}}
    if rels:
        body["data"]["relationships"] = rels
    asc.call("PATCH", f"/v1/{kind}/{id_}", body)


def state_of(item) -> str:
    a = item["attributes"]
    return a.get("appVersionState") or a.get("appStoreState") or a.get("state") or ""


def sync_app_info(asc, app_id, t):
    infos = asc.call("GET", f"/v1/apps/{app_id}/appInfos")["data"]
    info = next((i for i in infos if state_of(i) not in RELEASED), None)
    if not info:
        print("App Information: nothing editable (only a live version exists); skipped")
        return
    patch(asc, "appInfos", info["id"], {}, {
        "primaryCategory": {"data": {"type": "appCategories", "id": read("primary_category.txt")}},
        "secondaryCategory": {"data": {"type": "appCategories", "id": read("secondary_category.txt")}}})
    attrs = {"name": t["name"], "subtitle": t["subtitle"], "privacyPolicyUrl": t["privacy_url"]}
    loc = find_or_create(asc, f"/v1/appInfos/{info['id']}/appInfoLocalizations",
                         ("/v1/appInfoLocalizations", {"data": {"type": "appInfoLocalizations",
                          "attributes": {"locale": LOCALE, **attrs},
                          "relationships": {"appInfo": {"data": {"type": "appInfos", "id": info["id"]}}}}}),
                         lambda l: l["attributes"]["locale"] == LOCALE)
    patch(asc, "appInfoLocalizations", loc["id"], attrs)
    print(f"App Information: name, subtitle, privacy URL, categories set ({state_of(info)})")


def sync_version(asc, app_id, t):
    version = marketing_version()
    versions = asc.call("GET", f"/v1/apps/{app_id}/appStoreVersions" + q({"filter[platform]": "IOS", "limit": 200}))["data"]
    released = any(state_of(v) in RELEASED for v in versions)
    v = next((v for v in versions if v["attributes"]["versionString"] == version), None)
    if v and state_of(v) not in EDITABLE:
        print(f"version {version} is {state_of(v)}: not editable, listing skipped")
        return None
    if not v:
        v = next((v for v in versions if state_of(v) in EDITABLE), None)
        if v:
            print(f"renaming version {v['attributes']['versionString']} ({state_of(v)}) to {version}")
            patch(asc, "appStoreVersions", v["id"], {"versionString": version})
        else:
            print(f"creating version {version}")
            v = asc.call("POST", "/v1/appStoreVersions", {"data": {"type": "appStoreVersions",
                         "attributes": {"platform": "IOS", "versionString": version},
                         "relationships": {"app": {"data": {"type": "apps", "id": app_id}}}}})["data"]
    patch(asc, "appStoreVersions", v["id"], {"copyright": read("copyright.txt")})

    attrs = {"description": t["description"], "keywords": t["keywords"], "promotionalText": t["promotional_text"],
             "supportUrl": t["support_url"], "marketingUrl": t["marketing_url"]}
    if released:
        attrs["whatsNew"] = t["release_notes"]
    loc = find_or_create(asc, f"/v1/appStoreVersions/{v['id']}/appStoreVersionLocalizations",
                         ("/v1/appStoreVersionLocalizations", {"data": {"type": "appStoreVersionLocalizations",
                          "attributes": {"locale": LOCALE},
                          "relationships": {"appStoreVersion": {"data": {"type": "appStoreVersions", "id": v["id"]}}}}}),
                         lambda l: l["attributes"]["locale"] == LOCALE)
    patch(asc, "appStoreVersionLocalizations", loc["id"], attrs)
    print(f"version {version}: description, keywords, promo text, URLs, copyright set"
          + ("" if released else " (What's New skipped: first release)"))
    return loc


def sync_screenshots(asc, loc):
    files = screenshots()
    sets = asc.call("GET", f"/v1/appStoreVersionLocalizations/{loc['id']}/appScreenshotSets")["data"]
    sset = next((s for s in sets if s["attributes"]["screenshotDisplayType"] == DISPLAY_TYPE), None)
    if not sset:
        sset = asc.call("POST", "/v1/appScreenshotSets", {"data": {"type": "appScreenshotSets",
                        "attributes": {"screenshotDisplayType": DISPLAY_TYPE},
                        "relationships": {"appStoreVersionLocalization": {"data": {
                            "type": "appStoreVersionLocalizations", "id": loc["id"]}}}}})["data"]
    existing = asc.call("GET", f"/v1/appScreenshotSets/{sset['id']}/appScreenshots")["data"]
    want = [md5(f) for f in files]
    have = [s["attributes"].get("sourceFileChecksum") for s in existing]
    if have == want:
        print(f"screenshots: {len(files)} already up to date")
        return
    for s in existing:
        asc.call("DELETE", f"/v1/appScreenshots/{s['id']}")
    ids = []
    for path, checksum in zip(files, want):
        shot = asc.call("POST", "/v1/appScreenshots", {"data": {"type": "appScreenshots",
                        "attributes": {"fileName": os.path.basename(path), "fileSize": os.path.getsize(path)},
                        "relationships": {"appScreenshotSet": {"data": {"type": "appScreenshotSets", "id": sset["id"]}}}}})["data"]
        with open(path, "rb") as f:
            for op in shot["attributes"].get("uploadOperations") or []:
                f.seek(op.get("offset", 0))
                put_part(op, f.read(op["length"]))
        patch(asc, "appScreenshots", shot["id"], {"uploaded": True, "sourceFileChecksum": checksum})
        ids.append(shot["id"])
        print(f"  uploaded {os.path.basename(path)}", flush=True)
    for sid in ids:  # Apple processes each image; wait so failures show up here
        for _ in range(30):
            st = asc.call("GET", f"/v1/appScreenshots/{sid}")["data"]["attributes"].get("assetDeliveryState") or {}
            if st.get("state") == "COMPLETE":
                break
            if st.get("state") == "FAILED":
                raise SystemExit(f"screenshot {sid} failed: {st.get('errors')}")
            time.sleep(5)
        else:
            print(f"  screenshot {sid} still processing; it'll finish on Apple's side")
    asc.call("PATCH", f"/v1/appScreenshotSets/{sset['id']}/relationships/appScreenshots",
             {"data": [{"type": "appScreenshots", "id": i} for i in ids]})
    print(f"screenshots: replaced {len(existing)} with {len(ids)}, in file-name order")


def sync_beta(asc, app_id, t):
    locs = asc.call("GET", f"/v1/apps/{app_id}/betaAppLocalizations")["data"]
    loc = next((l for l in locs if l["attributes"]["locale"] == LOCALE), None)
    if loc:
        patch(asc, "betaAppLocalizations", loc["id"], {"description": t["beta_description"]})
    else:
        asc.call("POST", "/v1/betaAppLocalizations", {"data": {"type": "betaAppLocalizations",
                 "attributes": {"locale": LOCALE, "description": t["beta_description"]},
                 "relationships": {"app": {"data": {"type": "apps", "id": app_id}}}}})
    print("TestFlight: beta app description set")


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--check", action="store_true", help="only validate the files, offline")
    ap.add_argument("--bundle-id", default="app.shepherdr")
    a = ap.parse_args()
    t = check()
    if a.check:
        return
    asc = ASC(os.environ["ASC_KEY_PATH"], os.environ["ASC_KEY_ID"], os.environ["ASC_ISSUER_ID"])
    apps = asc.call("GET", "/v1/apps" + q({"filter[bundleId]": a.bundle_id}))["data"]
    if not apps:
        raise SystemExit(f"No App Store Connect app with bundle ID {a.bundle_id}")
    app_id = apps[0]["id"]
    sync_app_info(asc, app_id, t)
    loc = sync_version(asc, app_id, t)
    if loc:
        sync_screenshots(asc, loc)
    sync_beta(asc, app_id, t)
    print("App Store listing synced. Submitting for review stays manual.")


if __name__ == "__main__":
    main()
