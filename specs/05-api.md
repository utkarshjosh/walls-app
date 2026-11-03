# API specification and endpoint inventory

Status: proposed v0.4, updated 2026-11-03. Base path `/api/v1` at the proposed same-origin `https://walls.utkarshjoshi.com`. [OpenAPI](openapi.yaml) defines the device-sync slice; the rest of this inventory requires concrete payload schemas during its implementation stage. Do not generate an entire server from the partial file and mistake it for full product coverage.

## Authentication decision

Google is the selected login method. Firebase Auth or Supabase Auth will be chosen when authentication integration starts. Use a supported provider flow and server verification SDK rather than building a custom identity service. The auth-specific routes below are provisional sketches of required outcomes; map or replace them with the selected provider's endpoints/session adapter. They are not part of the validated sync OpenAPI and must not force a parallel custom OAuth/refresh system. UI prototyping uses local sample state.

## Conventions

JSON UTF-8; UUID identifiers; UTC RFC 3339 timestamps; integer bytes. `application/problem+json` errors contain `type`, `title`, `status`, `code`, `detail`, and `request_id`. Use 400 malformed input, 401 absent/invalid credential, 403 forbidden operation, 404 inaccessible/missing private resource, 409 conflicting state, 412 stale revision, 413 oversize, 422 invalid domain input, 429 throttling, and 503 dependency unavailable.

Target browser transport uses secure HttpOnly SameSite=Lax cookies, origin checks, and CSRF protection for mutations. The selected managed-auth integration must validate identity and callback binding using its supported flow/SDK; avoid handwritten token verification. Desktop user endpoints accept a user bearer credential through the native broker, validated by the managed-auth adapter. Finalize session exchange and logout behavior when the provider is chosen. Device endpoints require a device-scoped opaque token; they do not accept browser cookies or desktop user tokens. Never put reusable credentials in URLs or localStorage.

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

## Managed desktop authentication: integration gate

Choose one provider and prove Google sign-in through the system browser, return to the initiating app, server-side identity validation, session renewal/logout, and secure native storage. Bind callbacks to the initiating login attempt and validate redirect destinations; use PKCE/state/nonce as required by the supported flow. No provider client secret ships in the app. Desktop reusable credentials remain in Keychain/native memory.

Access/refresh lifetimes and rotation follow the chosen managed provider; the previous custom 15-minute/30-day session-family proposal is superseded. Keep stable internal account IDs so a provider change does not rewrite collection ownership. Firebase/Supabase selection here is for auth, not an implicit migration of PostgreSQL or OCI storage.

The device credential remains separately scoped, so interactive login expiry cannot turn it into a collection-edit or admin token. Device registration must not retain a plaintext credential in the generic 24-hour idempotency store: use bounded encrypted recovery, as for credential rotation, and explicit recovery after expiry.

Before production, document the selected provider configuration and concrete browser/native integration contracts, replace superseded auth route sketches, and test account isolation and revocation. UI prototypes need no live OAuth project; development sample state must not become a production bypass. Admin endpoint schemas also remain future work. The existing OpenAPI specifies only device sync.

## Versioning

Manifest schema version is independent of API path version. Add optional fields compatibly; clients ignore unknown fields but reject unsupported major schema versions, invalid required fields, duplicate entry IDs, invalid hashes, or oversized bodies. The v1 manifest maximum is 2 MiB. Schema validators can be stricter for authored fixtures than tolerant production decoders. Breaking changes require capability negotiation and a new major version, not a silent interpretation change.
