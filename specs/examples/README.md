# Contract examples

These are synthetic payloads for discussion and contract validation, not working downloads or a deployed service. UUIDs and SHA-256 strings are illustrative; no matching image binaries are included.

`schedule.json` defines hourly ordered playback and a 22:00 daily sync in Asia/Kolkata. It has no weekly overrides so the full corresponding `manifest.json` can be inspected without needing hidden playlist state. The manifest covers 2026-10-31 22:00 through 2026-11-07 22:00 in that timezone (seven days / 168 hourly entries), rotating three synthetic variants. A five-minute daily sync jitter is illustrated.

`device-state.json` shows the state check performed even if a subsequent manifest request returns 304. `download-grant.json` illustrates a separately expiring URL on a reserved example domain. No API bearer token belongs in an image URL.

JSON Schemas validate structure and formats. Domain checks must additionally enforce ownership, real IANA timezone names, non-overlapping windows, increasing UTC entries, variant membership, pixel count, and total byte budget. See [acceptance scenarios](../06-acceptance-scenarios.md).
