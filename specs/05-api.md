# API specification and endpoint inventory

Status: proposed v0.3. Base path `/api/v1` at the proposed same-origin `https://walls.utkarshjoshi.com`. [OpenAPI](openapi.yaml) defines the device-sync slice; the rest of this inventory requires concrete payload schemas during its implementation stage. Do not generate an entire server from the partial file and mistake it for full product coverage.

## Conventions

JSON UTF-8; UUID identifiers; UTC RFC 3339 timestamps; integer bytes. `application/problem+json` errors contain `type`, `title`, `status`, `code`, `detail`, and `request_id`. Use 400 malformed input, 401 absent/invalid credential, 403 forbidden operation, 404 inaccessible/missing private resource, 409 conflicting state, 412 stale revision, 413 oversize, 422 invalid domain input, 429 throttling, and 503 dependency unavailable.

Web uses secure HttpOnly SameSite=Lax cookies, authenticated OIDC server flow, origin checking, and CSRF tokens for mutations. OIDC callback validates state, issuer/audience, nonce, PKCE, signature, and redirect URI. Desktop user endpoints accept a separate opaque user bearer session through the native broker. Device endpoints require a device-scoped opaque token; they do not accept browser cookies or desktop user tokens. Never put reusable credentials in URLs or localStorage.

In the table, **User** means an authorized browser session or desktop user session. CSRF applies to cookie-authenticated writes; desktop bearer requests use fixed native operations and the same ownership/idempotency checks. Endpoints reject mixed credential classes. Device tokens never authorize library CRUD. Admin endpoints require a separate browser admin session and server role check; desktop user tokens cannot use them.

Collection/list reads use cursor pagination, default 30 / max 100. Opaque cursors include stable sort values and filter context; they do not bypass authorization. Private responses use appropriate private/no-store caching. Public catalog metadata can be briefly cached only after moderation filtering.

Create operations accept `Idempotency-Key`; persist user/device scope + method/path + key + canonical request hash and response for 24 hours. Same key and body returns prior result; same key with a different body returns 409. Resource edits use If-Match and monotonically increasing revisions. Device events additionally deduplicate permanent event IDs for the retained event window.

## Endpoint inventory

Core endpoints cover uploads, collections, starter onboarding, rotation preferences, and device sync. `/discover`, `/reports`, `/hidden`, and future publication submissions are deferred to the discovery phase. There are no MVP playlist, schedule-rule, or timeline-preview endpoints. The schedule JSON Schema describes persisted internal rotation preferences, not a setup form.

| Endpoint | Authentication | Behavior |
| --- | --- | --- |
| `GET /auth/login`, `GET /auth/callback` | OIDC redirect flow | Start/complete login and allowlist check |
| `POST /auth/logout` | Browser + CSRF | Revoke session |
| `POST /auth/desktop/start` | Public, throttled + PKCE challenge | Create expiring browser sign-in handoff bound to app/redirect |
| `POST /auth/desktop/token` | Single-use handoff code + PKCE verifier | Exchange for user access/refresh session; never a device token |
| `POST /auth/desktop/refresh` | Native refresh token | Rotate interactive session; detect reuse |
| `POST /auth/desktop/logout` | Desktop user | Revoke interactive session family |
| `POST /devices/register` | Desktop user + idempotency | Explicit Connect action issues one device token; enforce owner device limit |
| `GET /me` | User | Profile, quotas, interests |
| `PATCH /me/preferences` | User + CSRF + If-Match | Update explicit interests; inferred preferences are deferred |
| `POST /uploads` | User + CSRF + idempotency | Reserve quota, create upload intent |
| `PUT /uploads/{id}/content` | User + CSRF | Bounded JPEG/PNG stream to quarantine |
| `POST /uploads/{id}/complete` | User + CSRF + idempotency | Queue verification; return processing asset |
| `GET /assets/{id}` | User owner/catalog policy | Metadata and processing status |
| `GET /library`, `POST /library` | User; CSRF on write | List or save a permitted asset |
| `DELETE /library/{id}` | User + CSRF | Remove own reference and dependent memberships |
| `GET/POST /collections`, `PATCH/DELETE /collections/{id}` | User | Owner CRUD; CSRF on writes, If-Match on edits |
| `PUT/DELETE /collections/{id}/items/{item_id}` | User + CSRF | Idempotent owner membership change |
| `GET /starter-collections` | User | Small reviewed template set and interest tags; no live source dependency |
| `POST /onboarding` | User + CSRF + idempotency | Apply starter choices/interests, create private folders and default rotation atomically |
| `GET /rotation`, `PUT /rotation` | User; CSRF + If-Match on write | Singleton folder selection/mode/cadence; optional secondary sync window. Server manages ID/revision/timezone |
| `GET /devices`, `PATCH /devices/{id}` | User | Owner device status/name; initial rotation is assigned automatically |
| `DELETE /devices/{id}` | User + CSRF | Revoke device credentials |
| `POST /pairings` | Public, throttled | Create expiring device/user codes; name/platform input |
| `POST /pairings/approve` | Browser + CSRF | Approve entered user code for displayed device |
| `POST /pairings/token` | Pairing device code | Poll/one-time consume; 202 pending, 200 token, 410 expired/consumed |
| `POST /devices/self/credential` | Device + idempotency | Rotate credential with short old-token grace |
| `GET /devices/self/state` | Device | Full relevant blocked-variant state, server time, desired revision/status |
| `GET /devices/self/manifest` | Device + optional If-None-Match | 200 plan, 304 unchanged, 202 compiling |
| `POST /devices/self/download-grants` | Device | Authorize <=100 variant IDs from a retained plan; short-lived URLs |
| `POST /devices/self/events` | Device | <=100 deduplicated activation/state/error events |
| `GET /assets/{id}/thumbnail` | Owner or approved catalog | Authorize/serve thumbnail, never arbitrary object key |
| `POST /assets/{id}/download-grant` | User + CSRF | Authorize owner/catalog full-file download |
| `GET /discover` | Public | Approved catalog only; bounded tag/dimension/source filters |
| `POST /reports` | User + CSRF + throttling | Report catalog item |
| `PUT/DELETE /hidden/{asset_id}` | User + CSRF | Personal discovery exclusion |
| `POST /exports`, `GET /exports/{id}` | User | Async private export and expiring retrieval |
| `DELETE /me` | User + CSRF + recent reauthentication | Revoke and schedule account deletion |
| `GET /healthz`, `GET /readyz` | Minimal public info | Liveness and readiness without secrets |

Admin source, report, and moderation endpoints are restricted by role and audit logged; finalize schemas before shared beta. The browser verification page for pairing is a web route, not a public endpoint that performs approval by GET.

## Limits and credential lifecycle

Default personal limits: three paired devices, 1 GB logical managed library allowance bounded by aggregate availability, 25 MiB per upload, 60 MP decoded image, 100 cached rotation candidates per device plan, 100 entries per event/grant batch. Installation uses a proposed 7 GB intake / 8 GB hard ceiling under the owner-required 10 GB constraint; reserve input and output bytes before writing. See the [storage specification](09-storage-budget-and-integration.md). Operator caps may be lower based on effective shared quota usage. Rate limits distinguish unauthenticated pairing, signed-in writes, device sync, and public catalog reads.

Pairing codes expire in ten minutes and are single-use; retry/poll abuse is rate-limited. Device credential is random 256-bit data, hashed server-side, 90-day lifetime, normally rotated after 30 days during a sync. A token cannot enumerate other devices or access arbitrary owner images. An authenticated credential-rotation request is idempotent to recover a lost response; store its replay response encrypted for only the five-minute recovery window, then delete it. Replay after that window returns 409 and the client re-pairs if it has lost the new credential.

## Desktop user session contract to finalize

Use external browser authorization with PKCE S256, server-validated OIDC, and a short-lived single-use handoff. A loopback callback must bind the initiating app, state, and verifier; accept only an explicitly validated loopback return address/port, never arbitrary redirects. Reusable access/refresh tokens are returned only by native redemption and stored in Keychain. No client secret ships in the app.

Proposed lifetimes: handoff 60 seconds, opaque user access token 15 minutes, refresh family 30 days. Hash tokens server-side; rotate refresh tokens with bounded encrypted replay recovery, and revoke the family on proven reuse. These are independent of device-token rotation. Native logout revokes its user-session family; device disconnect revokes playback separately. Expired interactive login must not silently broaden a device token to keep collection writes working.

Device registration must not retain a plaintext credential in the generic 24-hour idempotency response store: use a bounded encrypted recovery response, as for credential rotation, and require explicit recovery after expiry.

Formal request/response/error schemas for this handoff, registration, refresh races, and admin endpoints are required before implementation. The existing OpenAPI still specifies only device sync.

## Versioning

Manifest schema version is independent of API path version. Add optional fields compatibly; clients ignore unknown fields but reject unsupported major schema versions, invalid required fields, duplicate entry IDs, invalid hashes, or oversized bodies. The v1 manifest maximum is 2 MiB. Schema validators can be stricter for authored fixtures than tolerant production decoders. Breaking changes require capability negotiation and a new major version, not a silent interpretation change.
