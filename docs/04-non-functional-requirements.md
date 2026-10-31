# Non-functional requirements

These are proposed test targets, not current measurements or an SLA. Measure after the macOS and storage spikes; adjust against actual server capacity.

| ID | Area | Target and verification |
| --- | --- | --- |
| NFR-01 | Ads/privacy | No ad SDKs, sponsored discovery ranking, or third-party behavioral analytics. Audit dependencies and network traffic before beta |
| NFR-02 | Availability | 99.5% monthly control-plane target for shared beta, measured by an external HTTPS probe. One primary database means no high-availability promise; offline playback absorbs server outages |
| NFR-03 | Offline | Demonstrate 72 hours with networking disabled; target seven days when selected assets fit the cache. At horizon expiry hold the current verified image and surface stale status |
| NFR-04 | API latency | At 10 simultaneous active users and 10,000 catalog items, warm metadata reads p95 <300 ms server time; writes p95 <500 ms excluding image processing and external OIDC calls |
| NFR-05 | Web | First usable library view <2 s on a 20 Mbps/50 ms link with 100 thumbnails; lazy-load images, cursor pagination, avoid original-image grid downloads |
| NFR-06 | Mac resources | On target Apple Silicon hardware: <1% average idle CPU over ten minutes, <100 MiB idle RSS, no periodic tight polling; downloads/decodes measured separately |
| NFR-07 | Timing | While app is running and Mac awake, apply due change within 10 s p95; after wake within 30 s. Sleep, shutdown, logout, and OS scheduling prevent a strict wall-clock guarantee |
| NFR-08 | Durability | Daily encrypted DB backup plus permitted-image backup; initial RPO 24 h, RTO 4 h from documented manual restore. Prove restore quarterly and before shared beta |
| NFR-09 | Security | TLS externally; private DB; least-privilege bucket credentials; OIDC state/nonce/PKCE validation; Keychain credentials; request authorization on every asset, collection, device, and plan |
| NFR-10 | Ingestion safety | Bounded bytes, pixels, duration, redirects, and parallelism. Isolate decoders; reject SVG/HTML/executable formats; prevent SSRF including metadata addresses and IPv6/private endpoints |
| NFR-11 | Isolation | Cross-account ID substitution tests for every private endpoint and object grant. Private blobs are not publicly enumerable, even when identical bytes exist in another account |
| NFR-12 | Compatibility | Proposed macOS 14+ minimum; explicitly test the owner's OS and current supported major release before committing the matrix. Universal build is a later distribution gate if Intel support is needed |
| NFR-13 | Operability | Structured logs, request/job IDs, health/readiness probes, queue age, source failures, sync age, cache coverage, storage usage, and backup age. No tokens, image EXIF, or signed URLs in logs |
| NFR-14 | Accessibility | Target WCAG 2.2 AA for key web flows; keyboard navigation, visible focus, labelled controls, non-drag reorder alternative, reduced motion, status not conveyed by color alone |
| NFR-15 | Portability | API/web/worker build for Linux arm64 and amd64; object-store contract avoids OCI-specific details outside its adapter; deployment uses Compose or systemd, not Kubernetes |
| NFR-16 | Maintainability | Module boundaries, SQL migrations, OpenAPI/schema checks, representative end-to-end tests, supported dependency versions pinned during implementation; no invented test coverage percentage gate |
| NFR-17 | Cost | Personal default: 5 GiB per-account managed library quota, 512 MiB Mac cache, no automatic paid capacity expansion. Installation caps must reflect live OCI tenancy allowances |
| NFR-18 | Retention | Operational logs 14 days; device event details 30 days; revoked tokens unusable immediately online; deleted user data purged from live storage within 7 days, backups age out within 30 days |
| NFR-19 | Simplicity | Complete preference selection in under two minutes and at most three onboarding steps, excluding auth/install/transfers. No schedule editor required; healthy operation generates no routine notifications. Verify with first-use trials |

Performance budgets exclude arbitrary originals; 25 MiB and 60 MP are hard intake ceilings, not typical files. Limit worker concurrency to one initially, with a 1 GiB memory ceiling and timeout. Benchmark that ceiling using worst-case allowed images before opening uploads beyond the owner.

Cached public images already downloaded to a disconnected device cannot be remotely erased immediately. On reconnection the client enforces tombstones. Account deletion and source withdrawal documentation must acknowledge this limit.
