# Contract examples

These are synthetic payloads for discussion and contract validation, not working downloads or a deployed service. UUIDs and SHA-256 strings are illustrative; no matching image binaries are included.

`schedule.json` illustrates the optional In order mode with two connected collections and hourly cadence; onboarding defaults to Shuffle. Collection A contains variants 10/11 and B contains 11/12. Their deduplicated union is 10/11/12, with first-occurrence collection provenance. The matching seven-day manifest has 168 hourly entries from 2026-10-31 22:00 to 2026-11-07 22:00 in Asia/Kolkata. Night sync is automatic; a five-minute jitter is illustrated. There is no separate playlist or weekly rule configuration.

`device-state.json` includes whether rotation is enabled and shows the state check performed even if a subsequent manifest request returns 304. `download-grant.json` illustrates a separately expiring URL on a reserved example domain. No API bearer token belongs in an image URL.

JSON Schemas validate structure and formats. Domain checks must additionally enforce ownership, real IANA timezone names, unique owned collection references, increasing UTC entries, variant membership, pixel count, and total byte budget. See [acceptance scenarios](../06-acceptance-scenarios.md).
