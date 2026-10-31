# Oracle deployment, storage, and operating cost

## Known inputs and assumptions

Two existing Oracle servers and `utkarshjoshi.com` are available. Supplied sizing: **ARM processor, 2 cores, 12 GB RAM**. It is not yet confirmed whether this is per machine or the combined allocation. OS, disk capacity, free space, region, network layout, Always Free/PAYG status, and other workloads are unknown. No server was accessed and no DNS changes were made during planning.

Use the stronger/less-loaded machine as A. Target a combined steady-state footprint below 4 GiB RAM for API, DB, reverse proxy, and one bounded worker before scaling. Build containers/binaries in CI or on a development machine, not on a production VM during normal operation.

## Proposed placement

| Location | Responsibilities |
| --- | --- |
| VM A | Caddy, static web assets, Go API, PostgreSQL primary, scheduled backup coordinator |
| VM B | Go worker, image normalization, approved source ingestion, restore rehearsal tools; temporary processing files only |
| OCI Object Storage | Private originals and variants; separate backup bucket and credentials |
| Mac | Keychain device token, local state, verified wallpaper cache, native scheduling |

VM B reaches PostgreSQL over a private VCN route (or authenticated private tunnel if servers are in different networks). If that connection is unreliable, co-locate the worker on A and use B for restore/maintenance until fixed. Do not expose PostgreSQL on public interfaces. Two servers do not make this topology highly available: A and the database remain a single point of failure.

Proposed DNS: `walls.utkarshjoshi.com` → A. Serve web and `/api/v1` from one origin. Object transfers use provider URLs returned by the backend; `media.utkarshjoshi.com` and a CDN are optional later. Keep other services on the root domain untouched. TLS terminates at Caddy; inbound rules allow HTTPS and tightly restricted administration access.

## Blob-store decision

Start with **OCI Object Storage Standard tier**, private buckets, encryption at rest, and an S3-compatible adapter. Confirm PUT/GET/HEAD/DELETE, expiry-constrained GET signing, checksums, CORS, and range behavior against the real tenancy in the first spike. OCI documents implementation differences and Customer Secret Keys for its S3 interface. [OCI S3 documentation](https://docs.oracle.com/en-us/iaas/Content/Object/Tasks/s3compatibleapi.htm)

Recommended adapter operations are `put`, `head`, `open`, `delete`, and `grant_download`; do not scatter provider URLs or bucket names through domain models. Server-side scoped credentials never reach clients. Object grants expire in 15 minutes; durable manifests contain variant IDs and hashes, not expiring URLs. An authenticated client asks for fresh grants when it is actually downloading.

Do not initially run MinIO/distributed storage: two small machines add maintenance and failure coupling without establishing a proper durable cluster. Local filesystem storage can support development but is not the only production copy. Re-evaluate alternative S3 providers only if actual OCI limits or costs become unfavorable.

## Capacity model

Illustrative assumptions, **not provider prices**: 6 MiB original + 3 MiB desktop derivative + 0.15 MiB thumbnail = 9.15 MiB per wallpaper. Derivative sizes vary significantly with content; measure a representative sample before importing a large collection.

| Library | Managed image bytes, one copy | Implication |
| --- | --- | --- |
| 500 wallpapers | ~4.47 GiB | Good personal starting size |
| 2,000 wallpapers | ~17.87 GiB | Already close to a 20 GB allowance before backups/versioning |
| 10,000 wallpapers | ~89.36 GiB | Budget paid storage or tighten retention |

One Mac fetching 24 new 3 MiB derivatives daily uses roughly **2.11 GiB per 30 days**. At 100 similarly active Macs this is about **211 GiB/month**, excluding thumbnails, originals, retries, and backups. A 512 MiB cache can hold about 100 such derivatives plus headroom; playlist repetition allows many more scheduled events than unique files.

Request volume can dominate free allowances: 100 Macs × 24 object GETs × 30 days = **72,000 GETs/month**, even before web thumbnails and uploads. Track storage, requests, and transfer independently. Budgets notify; they are not assumed to be automatic spending caps. Implement application quotas and operator circuit breakers.

## Oracle allowances: verify the tenancy

The official page reviewed on 2026-10-31 lists 20 GB combined object storage and 50,000 monthly object requests for Always Free-only accounts; paid/trial accounts have a different tier split. It also lists 10 TB monthly outbound transfer, shared boot/block-volume allowances, and possible idle-instance reclamation. These are tenancy-wide allowances, not doubled by having two VMs. [Oracle Always Free resources](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm)

Do not promise a zero bill from those headline numbers. Inspect current Limits/Quotas/Usage and existing consumption. Standard, Infrequent Access, and Archive are not interchangeable for frequently fetched wallpaper bytes. Backups, retained object versions, and deleted versions count toward actual consumption. Prices and signing/distribution expenses need confirmation before public release.

## Backup and recovery

Daily: encrypted PostgreSQL dump, manifest/source/deletion records, object inventory, and incremental backup of owned/permitted images to a separately permissioned destination. Keep seven daily and four weekly recovery points within the thirty-day deletion policy; do not blindly duplicate every blob for every snapshot. Configure version expiration and backup quotas deliberately.

An object bucket is primary storage, not by itself a complete backup. A second bucket in the same tenancy protects against some application mistakes but not loss of the account. Before shared beta, keep an encrypted offline or independent-account copy of irreplaceable personal files and metadata. Record recovery-key custody outside the servers.

Restore drill: provision clean services; restore DB and blobs; replay deletion/withdrawal records; verify sampled checksums and object inventory; pair a test client; confirm offline playback; then switch traffic. Target RPO 24 hours and RTO four hours. Do not auto-promote VM B as a DB primary without an explicit replication/failover design.

## Release and operations

Pin ARM64-compatible image digests after validation. Use non-root containers/processes, read-only application filesystems where practical, worker memory/CPU limits, health checks, and automatic service restart. Run migrations once with a designated release job. Keep the previous application artifact for rollback and distinguish reversible app rollback from schema rollback.

Alert on failed backup, queue age >15 minutes, repeated decoder failures, missing scheduled compilation, object usage >70/85/95% of configured cap, and client contact older than 48 hours. A deliberately sleeping/disconnected Mac is a status signal, not necessarily an incident.
