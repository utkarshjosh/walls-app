# Oracle deployment, storage, and operating cost

## Known inputs and assumptions

Two usable Oracle servers and `utkarshjoshi.com` are available. On 2026-11-03 the owner clarified that **ARM processor, 2 cores, 12 GB RAM** describes one server rather than a combined allocation. The SSH aliases are **`oracle`**, which hosts active applications, and **`agent-server-oci`**, which is mostly empty. Do not infer that both machines have identical sizing; verify each host's CPU/RAM, OS, disk space, region, and tenancy status during integration. No server was accessed and no DNS changes were made during this planning update.

Update 2026-11-01: the owner created **`walls-assets`**, **private**, in the **root compartment**, and reports a rule intended to cap usage at **8 GB**, with a requirement to stay below **10 GB**. Preserve this stricter budget regardless of generic provider allowances. The rule, bucket tier, credentials, and network are not independently verified. Firewall and integration tests are explicitly deferred; see the [budget/integration specification](../specs/09-storage-budget-and-integration.md).

Prefer **`agent-server-oci`** for the first deployment. Target a steady-state footprint below 4 GiB RAM for API, DB, reverse proxy, and one bounded worker on this host, subject to inventory and measurement. Build containers/binaries in CI or on a development machine, not on a production VM during normal operation.

## Proposed placement

| Location | Responsibilities |
| --- | --- |
| `agent-server-oci` | Preferred primary: Caddy, static consumer/admin web assets, Go API, PostgreSQL, one bounded worker, scheduled backup coordinator |
| `oracle` | Existing applications; no wallpaper workload assigned initially. Consider a specific maintenance/backup role later only after inventory |
| OCI Object Storage | Existing private `walls-assets` originals and variants; backup location independently budgeted |
| Mac | Bundled collection UI, Keychain user/device credentials, local state, verified cache, native scheduling |

Keep API, database, and worker together initially to avoid a cross-server database dependency. Bind PostgreSQL to a private local/container network. The worker remains a separately bounded process, even on the same machine. Split it onto another host only if measured load warrants it and private connectivity is verified. This is a single-primary deployment; merely having another server does not make it highly available.

Proposed DNS: `walls.utkarshjoshi.com` → `agent-server-oci`. Serve web and `/api/v1` from one origin. Object transfers use provider URLs returned by the backend; `media.utkarshjoshi.com` and a CDN are optional later. Keep other services on the root domain untouched. TLS terminates at Caddy; inbound rules allow HTTPS and tightly restricted administration access.

## Blob-store decision

Use the existing private **`walls-assets`** bucket; verify it is **Standard tier** before integrating. Keep encryption at rest and a narrow object-store adapter. Confirm PUT/GET/HEAD/DELETE, expiry-constrained GET signing, checksums, CORS, and range behavior during the deferred integration spike. OCI documents implementation differences and Customer Secret Keys for its S3 interface. [OCI S3 documentation](https://docs.oracle.com/en-us/iaas/Content/Object/Tasks/s3compatibleapi.htm)

Recommended adapter operations are `put`, `head`, `open`, `delete`, and `grant_download`; do not scatter provider URLs or bucket names through domain models. Server-side scoped credentials never reach clients. Object grants expire in 15 minutes; durable manifests contain variant IDs and hashes, not expiring URLs. An authenticated client asks for fresh grants when it is actually downloading.

Do not initially run MinIO/distributed storage: two small machines add maintenance and failure coupling without establishing a proper durable cluster. Local filesystem storage can support development but is not the only production copy. Re-evaluate alternative S3 providers only if actual OCI limits or costs become unfavorable.

## Capacity model

Illustrative assumptions, **not provider prices**: 6 MiB original + 3 MiB desktop derivative + 0.15 MiB thumbnail = 9.15 MiB per wallpaper. Derivative sizes vary significantly with content; measure a representative sample before importing a large collection.

| Library | Managed image bytes, one copy | Implication |
| --- | --- | --- |
| 500 wallpapers | ~4.47 GiB / 4.80 GB | Fits current budget before processing/backups |
| 700 wallpapers | ~6.25 GiB / 6.72 GB | Near the proposed 7 GB intake ceiling |
| 2,000 wallpapers | ~17.87 GiB / 19.19 GB | Exceeds this installation's budget; not an allowed growth target |

Proposed ceilings are **7,000,000,000 bytes** for normal intake and **8,000,000,000 bytes** hard, reduced by other usage covered by the effective quota. Reserve bytes transactionally for originals and derivatives before writes. Count thumbnails, quarantine, multipart leftovers, retained versions, and any backups. Default new-account allowance is 1 GB logical bytes, bounded by aggregate availability; the previous 5 GiB per-account default is superseded. Keep installer releases outside the image bucket.

One Mac fetching 24 new 3 MiB derivatives daily uses roughly **2.11 GiB per 30 days**. At 100 similarly active Macs this is about **211 GiB/month**, excluding thumbnails, originals, retries, and backups. A 512 MiB cache can hold about 100 such derivatives plus headroom; cached-cycle repetition allows many more scheduled events than unique files.

Request volume can dominate free allowances: 100 Macs × 24 object GETs × 30 days = **72,000 GETs/month**, even before web thumbnails and uploads. Track storage, requests, and transfer independently. Budgets notify; they are not assumed to be automatic spending caps. Implement application quotas and operator circuit breakers.

## Oracle allowances: verify the tenancy

The official page reviewed on 2026-10-31 lists 20 GB combined object storage and 50,000 monthly object requests for Always Free-only accounts; paid/trial accounts have a different tier split. It also lists 10 TB monthly outbound transfer, shared boot/block-volume allowances, and possible idle-instance reclamation. These are tenancy-wide allowances, not doubled by having two VMs. [Oracle Always Free resources](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm)

Do not promise a zero bill from those headline numbers or use them to loosen the owner's <10 GB constraint. Inspect current Limits/Quotas/Usage and existing consumption. Standard, Infrequent Access, and Archive are not interchangeable for frequently fetched wallpaper bytes. Retained versions and backups consume storage. OCI's storage quota is regional and supports compartment/tenancy policies; verify the owner's rule rather than treating it as a bucket-only cap. [OCI quotas](https://docs.oracle.com/en-us/iaas/Content/Quotas/Concepts/resourcequotas_topic-Object_Storage_Quotas.htm)

## Backup and recovery

Daily: encrypted PostgreSQL dump, manifest/source/deletion records, object inventory, and incremental backup of owned/permitted images to a separately permissioned destination. Keep seven daily and four weekly recovery points within the thirty-day deletion policy; do not blindly duplicate every blob for every snapshot. Configure version expiration and backup quotas deliberately.

An object bucket is primary storage, not by itself a complete backup. Do not create a second bucket assuming another free allowance. Prefer an encrypted owner-controlled offline or independent-account copy of irreplaceable personal files and metadata, with its capacity explicitly budgeted. A same-tenancy backup does not protect against loss of the account. Record recovery-key custody outside the servers.

Restore drill: provision clean services; restore DB and blobs; replay deletion/withdrawal records; verify sampled checksums and object inventory; pair a test client; confirm offline playback; then switch traffic. Target RPO 24 hours and RTO four hours. Do not auto-promote `oracle` as a DB primary without an explicit replication/failover design and assessment of its existing applications.

Operator limits, telemetry, and recovery procedures below are implementation responsibilities, not screens for everyday wallpaper users. Starter collections require only a small reviewed seed set; no model inference infrastructure is budgeted for MVP.

## Release and operations

Pin ARM64-compatible image digests after validation. Use non-root containers/processes, read-only application filesystems where practical, worker memory/CPU limits, health checks, and automatic service restart. Run migrations once with a designated release job. Keep the previous application artifact for rollback and distinguish reversible app rollback from schema rollback.

Alert on failed backup, queue age >15 minutes, repeated decoder failures, missing scheduled compilation, object usage >70/85/95% of configured cap, and client contact older than 48 hours. A deliberately sleeping/disconnected Mac is a status signal, not necessarily an incident.
