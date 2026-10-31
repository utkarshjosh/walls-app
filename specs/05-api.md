# API specification and endpoint inventory

Status: proposed v0.1. Base path `/api/v1` at the proposed same-origin `https://walls.utkarshjoshi.com`. [OpenAPI](openapi.yaml) defines the device-sync slice; the rest of this inventory requires concrete payload schemas during its implementation stage. Do not generate an entire server from the partial file and mistake it for full product coverage.

## Conventions

JSON UTF-8; UUID identifiers; UTC RFC 3339 timestamps; integer bytes. `application/problem+json` errors contain `type`, `title`, `status`, `code`, `detail`, and `request_id`. Use 400 malformed input, 401 absent/invalid credential, 403 forbidden operation, 404 inaccessible/missing private resource, 409 conflicting state, 412 stale revision, 413 oversize, 422 invalid domain input, 429 throttling, and 503 dependency unavailable.

Web uses secure HttpOnly SameSite=Lax cookies, authenticated OIDC server flow, origin checking, and CSRF tokens for mutations. OIDC callback validates state, issuer/audience, nonce, PKCE, signature, and redirect URI. Device endpoints require an opaque bearer token; they do not accept browser cookies. Never put either credential in URLs or localStorage.

Collection/list reads use cursor pagination, default 30 / max 100. Opaque cursors include stable sort values and filter context; they do not bypass authorization. Private responses use appropriate private/no-store caching. Public catalog metadata can be briefly cached only after moderation filtering.

Create operations accept `Idempotency-Key`; persist user/device scope + method/path + key + canonical request hash and response for 24 hours. Same key and body returns prior result; same key with a different body returns 409. Resource edits use If-Match and monotonically increasing revisions. Device events additionally deduplicate permanent event IDs for the retained event window.

## Endpoint inventory

| Endpoint | Authentication | Behavior |
| --- | --- | --- |
| `GET /auth/login`, `GET /auth/callback` | OIDC redirect flow | Start/complete login and allowlist check |
| `POST /auth/logout` | Web + CSRF | Revoke session |
| `GET /me` | Web | Profile, quotas, interests |
| `PATCH /me/preferences` | Web + CSRF + If-Match | Update interests/time defaults |
| `POST /uploads` | Web + CSRF + idempotency | Reserve quota, create upload intent |
| `PUT /uploads/{id}/content` | Web + CSRF | Bounded JPEG/PNG stream to quarantine |
| `POST /uploads/{id}/complete` | Web + CSRF + idempotency | Queue verification; return processing asset |
| `GET /assets/{id}` | Web owner/catalog policy | Metadata and processing status |
| `GET /library`, `POST /library` | Web; CSRF on write | List or save a permitted asset |
| `DELETE /library/{id}` | Web + CSRF | Remove own reference and dependent memberships |
| `GET/POST /collections`, `PATCH/DELETE /collections/{id}` | Web | Owner CRUD; CSRF on writes, If-Match on edits |
| `PUT/DELETE /collections/{id}/items/{item_id}` | Web + CSRF | Idempotent owner membership change |
| `GET/POST /playlists`, `GET/PATCH/DELETE /playlists/{id}` | Web | Owner playlist operations, revision checks |
| `PUT /playlists/{id}/items` | Web + CSRF + If-Match | Atomic complete item-order replacement |
| `GET/POST /schedules`, `PUT /schedules/{id}` | Web | Validate schema/domain; publish desired revision |
| `POST /schedules/preview` | Web + CSRF | Compile a bounded preview without saving |
| `GET /devices`, `PATCH /devices/{id}` | Web | Owner device status/schedule assignment |
| `DELETE /devices/{id}` | Web + CSRF | Revoke device credentials |
| `POST /pairings` | Public, throttled | Create expiring device/user codes; name/platform input |
| `POST /pairings/approve` | Web + CSRF | Approve entered user code for displayed device |
| `POST /pairings/token` | Pairing device code | Poll/one-time consume; 202 pending, 200 token, 410 expired/consumed |
| `POST /devices/self/credential` | Device + idempotency | Rotate credential with short old-token grace |
| `GET /devices/self/state` | Device | Full relevant blocked-variant state, server time, desired revision/status |
| `GET /devices/self/manifest` | Device + optional If-None-Match | 200 plan, 304 unchanged, 202 compiling |
| `POST /devices/self/download-grants` | Device | Authorize <=100 variant IDs from a retained plan; short-lived URLs |
| `POST /devices/self/events` | Device | <=100 deduplicated activation/state/error events |
| `GET /assets/{id}/thumbnail` | Owner or approved catalog | Authorize/serve thumbnail, never arbitrary object key |
| `POST /assets/{id}/download-grant` | Web + CSRF | Authorize owner/catalog full-file download |
| `GET /discover` | Public | Approved catalog only; bounded tag/dimension/source filters |
| `POST /reports` | Web + CSRF + throttling | Report catalog item |
| `PUT/DELETE /hidden/{asset_id}` | Web + CSRF | Personal discovery exclusion |
| `POST /exports`, `GET /exports/{id}` | Web | Async private export and expiring retrieval |
| `DELETE /me` | Web + CSRF + recent reauthentication | Revoke and schedule account deletion |
| `GET /healthz`, `GET /readyz` | Minimal public info | Liveness and readiness without secrets |

Admin source, report, and moderation endpoints are restricted by role and audit logged; finalize schemas before shared beta. The browser verification page for pairing is a web route, not a public endpoint that performs approval by GET.

## Limits and credential lifecycle

Default personal limits: three paired devices, 5 GiB managed library quota, 25 MiB per upload, 60 MP decoded image, 100 playlist candidates per device plan, 100 entries per event/grant batch. Operator caps may be lower based on tenancy capacity. Rate limits distinguish unauthenticated pairing, signed-in writes, device sync, and public catalog reads.

Pairing codes expire in ten minutes and are single-use; retry/poll abuse is rate-limited. Device credential is random 256-bit data, hashed server-side, 90-day lifetime, normally rotated after 30 days during a sync. A token cannot enumerate other devices or access arbitrary owner images. An authenticated rotation request is idempotent to recover a lost response; store its replay response encrypted for only the five-minute recovery window, then delete it. Replay after that window returns 409 and the client re-pairs if it has lost the new credential.

## Versioning

Manifest schema version is independent of API path version. Add optional fields compatibly; clients ignore unknown fields but reject unsupported major schema versions, invalid required fields, duplicate entry IDs, invalid hashes, or oversized bodies. The v1 manifest maximum is 2 MiB. Schema validators can be stricter for authored fixtures than tolerant production decoders. Breaking changes require capability negotiation and a new major version, not a silent interpretation change.
