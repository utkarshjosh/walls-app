"""Validate authored contracts/examples and local documentation links.

This checks planning artifacts, not an implementation of the application.
Run with Python 3.11+ after installing requirements-specs.txt.
"""

import copy
import json
import re
from datetime import datetime, timedelta
from pathlib import Path
from zoneinfo import ZoneInfo

import yaml
from jsonschema import Draft202012Validator, FormatChecker, ValidationError
from openapi_spec_validator import validate

ROOT = Path(__file__).resolve().parents[1]
FORMATS = FormatChecker()


def require(condition, message):
    if not condition:
        raise ValueError(message)


def read_json(relative):
    return json.loads((ROOT / relative).read_text())


def timestamp(value):
    require(value.endswith("Z"), f"Expected UTC timestamp: {value}")
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def schedule_semantics(schedule):
    ZoneInfo(schedule["timezone"])
    ids = [window["id"] for window in schedule["windows"]]
    require(len(ids) == len(set(ids)), "Duplicate window IDs")
    by_day = {day: [] for day in range(1, 8)}
    for window in schedule["windows"]:
        start = sum(int(v) * scale for v, scale in zip(window["start"].split(":"), [60, 1]))
        end = sum(int(v) * scale for v, scale in zip(window["end"].split(":"), [60, 1]))
        require(start < end, "Overnight/empty window must be split or rejected")
        for day in window["days"]:
            by_day[day].append((start, end))
    for windows in by_day.values():
        windows.sort()
        for previous, current in zip(windows, windows[1:]):
            require(previous[1] <= current[0], "Overlapping weekly windows")


def manifest_semantics(manifest):
    ZoneInfo(manifest["timezone"])
    begin, end = timestamp(manifest["valid_from"]), timestamp(manifest["valid_until"])
    require(timedelta(0) < end - begin <= timedelta(days=7), "Invalid manifest horizon")
    require(timestamp(manifest["generated_at"]) <= begin, "Generation must precede validity")
    require(begin < timestamp(manifest["next_sync_at"]) < end, "Next sync outside coverage")
    variants = {item["id"]: item for item in manifest["variants"]}
    require(len(variants) == len(manifest["variants"]), "Duplicate variant IDs")
    entries = manifest["entries"]
    require(len({item["id"] for item in entries}) == len(entries), "Duplicate entry IDs")
    times = [timestamp(item["apply_at"]) for item in entries]
    require(times[0] == begin, "First entry must start at valid_from")
    require(all(begin <= moment < end for moment in times), "Entry outside coverage")
    require(all(a < b for a, b in zip(times, times[1:])), "Entries must strictly increase")
    for entry in entries:
        require(entry["variant_id"] in variants, "Entry references missing variant")
        require(entry["playlist_id"] in manifest["playlist_revisions"], "Missing playlist revision")
    for variant in variants.values():
        require(variant["width"] * variant["height"] <= 60_000_000, "Pixel cap exceeded")


def check_links():
    count = 0
    for path in ROOT.rglob("*.md"):
        if ".git" in path.parts:
            continue
        # Authored local links are simple paths; web URLs and anchors are excluded.
        for target in re.findall(r"\]\(([^)]+)\)", path.read_text()):
            if target.startswith(("https://", "http://", "#", "mailto:")):
                continue
            target = target.split("#", 1)[0].strip("<>")
            require((path.parent / target).is_file(), f"Broken local link in {path}: {target}")
            count += 1
    return count


def expect_failure(check, value, label):
    try:
        check(value)
    except (ValueError, ValidationError):
        return
    raise ValueError(f"Invalid fixture accepted: {label}")


def main():
    validators = {}
    for name in ("schedule", "manifest"):
        schema = read_json(f"specs/schemas/{name}.schema.json")
        Draft202012Validator.check_schema(schema)
        validators[name] = Draft202012Validator(schema, format_checker=FORMATS)

    schedule = read_json("specs/examples/schedule.json")
    manifest = read_json("specs/examples/manifest.json")
    validators["schedule"].validate(schedule)
    validators["manifest"].validate(manifest)
    schedule_semantics(schedule)
    manifest_semantics(manifest)
    require(manifest["schedule_id"] == schedule["id"], "Fixture schedule mismatch")
    require(manifest["schedule_revision"] == schedule["revision"], "Fixture revision mismatch")
    require(len(manifest["entries"]) == 168, "Example should cover seven hourly days")
    require(sum(v["byte_size"] for v in manifest["variants"]) <= 0.4 * 512 * 1024**2,
            "Example does not fit default per-generation budget")

    api_path = ROOT / "specs/openapi.yaml"
    api = yaml.safe_load(api_path.read_text())
    validate(api, base_uri=api_path.as_uri())
    for name, schema_name in (("device-state", "DeviceState"), ("download-grant", "GrantResponse")):
        Draft202012Validator(api["components"]["schemas"][schema_name], format_checker=FORMATS).validate(
            read_json(f"specs/examples/{name}.json")
        )

    bad_interval = copy.deepcopy(schedule)
    bad_interval["default_interval_minutes"] = 0
    expect_failure(validators["schedule"].validate, bad_interval, "zero interval")
    bad_hash = copy.deepcopy(manifest)
    bad_hash["variants"][0]["sha256"] = "not-a-sha256"
    expect_failure(validators["manifest"].validate, bad_hash, "invalid checksum")
    bad_time = copy.deepcopy(manifest)
    bad_time["entries"][1]["apply_at"] = bad_time["entries"][0]["apply_at"]
    expect_failure(manifest_semantics, bad_time, "duplicate UTC instant")
    bad_reference = copy.deepcopy(manifest)
    bad_reference["entries"][0]["variant_id"] = "ffffffff-ffff-4fff-8fff-ffffffffffff"
    expect_failure(manifest_semantics, bad_reference, "missing variant")

    print("PASS: two Draft 2020-12 schemas, OpenAPI 3.1, four examples, domain invariants,")
    print(f"four negative fixtures, and {check_links()} local Markdown links.")
    print("Application behavior, native APIs, external links, and deployment are not tested here.")


if __name__ == "__main__":
    main()
