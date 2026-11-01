# Storage budget and deferred OCI integration

Proposed v0.1, 2026-11-01. The owner reports an existing **private `walls-assets` bucket in the root compartment**, a rule intended to cap storage at **8 GB**, and a requirement to stay below **10 GB**. These are supplied configuration facts, not independently verified integration results. No cloud settings were changed.

## Budget contract

Use decimal bytes for these operator limits: 7 GB = 7,000,000,000; 8 GB = 8,000,000,000; 10 GB = 10,000,000,000. GiB-based values are different. Before deployment, inspect the actual rule text, region, scope, effective limits, storage tier, and existing usage; the root compartment may contain unrelated objects.

OCI documents `object-storage` / `storage-bytes` as a regional quota and supports tenancy/compartment policies. Do not assume the owner's rule applies only to this bucket, or that a budget notification prevents writes. Check the precise policy and enforcement during integration. [OCI object quotas](https://docs.oracle.com/en-us/iaas/Content/Quotas/Concepts/resourcequotas_topic-Object_Storage_Quotas.htm)

Proposed application policy: **7 GB normal intake ceiling** and **8 GB hard ceiling**, reduced by any other usage under the verified quota. Keep the remaining headroom for in-flight processing, cleanup lag, and unexpected version/multipart bytes. The overall tenancy constraint remains below 10 GB; no automatic upgrades or spillover bucket. Pause intake if usage cannot be reconciled reliably. Reads and cached playback continue.

Default new-account allowance: **1 GB logical managed bytes**, within the shared installation limit. The owner may raise an account allowance within measured headroom; quotas are not allocations. Shared starter bytes count once physically, while each account's logical entitlement can still be charged consistently. Do not sum per-account allowances and assume that storage exists.

## Reservation and reconciliation

1. Before accepting upload bytes, lock the aggregate quota ledger and account ledger in one transaction. Reserve a bounded original plus bounded derivative allowance; do not trust client-reported size. Enforce actual stream length and encoder output ceilings.
2. Count originals, normalized desktop variants, thumbnails, quarantine, and unfinished uploads. Reserve output bytes before a worker writes them. If an encoder exceeds its cap, recompress within policy or fail cleanly; never publish an unaccounted object.
3. Keep reservations until real objects are deleted or converted to measured committed bytes. A cleanup failure does not free capacity on paper. Expire abandoned uploads after 24 hours and reconcile multipart leftovers and retained versions.
4. Reconcile inventory/usage against the ledger daily and after storage errors. Count any backup objects sharing the quota. Fail closed for new intake on unknown/over-budget usage and alert the operator; never delete private originals merely to make space.
5. Handle provider quota failures without retry storms: mark upload retryable after capacity recovery, clean up partial objects, retain previous plans, and expose a concise user-facing capacity message.

Disable unnecessary object versioning/retention in the proposed bucket policy only after checking recovery needs; do not change existing settings blindly. Keep installation packages on project release hosting. Budget backups independently; a second OCI bucket is not extra free capacity.

At an illustrative 6 MiB original + 3 MiB desktop + 0.15 MiB thumbnail, 500 wallpapers consume about **4.80 GB** before temporary files and backups. 700 consume about **6.72 GB**. These are examples, not guaranteed capacities; 2,000 would exceed this installation's ceiling. Measure actual images before seeding the library.

## Integration checklist for later

Record namespace, region, bucket tier, compartment OCID, server OS/network topology, and credential strategy in operator configuration. Keep keys outside Git and never paste private keys into design docs. Prefer instance principals if the chosen OCI adapter supports the required operations; otherwise use a dedicated least-privilege S3 credential and document rotation. Verify signed download behavior on the real adapter.

| Check | Evidence required |
| --- | --- |
| Private bucket | Unauthenticated object read/list denied; authorized short-lived grant works and expires |
| Least privilege | Application identity can access only intended bucket/prefix operations; cannot manage unrelated resources |
| Small round trip | Upload a small fixture, HEAD/download, verify hash, delete, and confirm ledger cleanup |
| Quota semantics | Inspect effective rule/bytes/scope; exercise app boundary with a low test cap, not by filling production to 8 GB |
| Network | Public HTTPS reachable; PostgreSQL and worker interfaces private; administration restricted; VCN/NSG and host firewall agree |
| CORS | Exact deployed web origins for browser transfers; no broad wildcard used to make desktop integration work |
| Desktop | Native HTTPS broker downloads, verifies, and caches without exposing storage credentials to the webview |
| Failure | Expired grants, denied IAM, network timeout, quota rejection, failed cleanup, and restart preserve working playback |
| Recovery | Independent backup copy and restore drill, with storage consumption included in the chosen budget |

The owner explicitly deferred firewall/network integration. This document is the test plan; no row has passed yet.
