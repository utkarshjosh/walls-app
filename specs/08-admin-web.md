# Minimal admin web specification

Proposed v0.1, 2026-11-01. The owner needs a browser administration surface even when normal users manage everything in the desktop app.

## Scope

Serve `/admin` from the service origin with a separate frontend entry point. MVP includes owner/invite access, account/device revocation, aggregate object usage and reservations, upload/processing failures, worker/backup health, and an intake pause switch. Present simple lists and actionable exceptions. No analytics suite, CRM, billing console, source crawler, or AI operations UI.

Provider network/IAM setup remains in deployment runbooks; the app is not an OCI console replacement. Display measured usage and last reconciliation time. Do not label a configured 8 GB ceiling as a verified cloud quota until the operator has checked its effective scope.

## Access and actions

Use browser sessions with CSRF protection and a server-checked admin role on every endpoint. Consumer desktop sessions and device tokens cannot call admin APIs, even if their owner has an admin role. Require a separate browser admin session and recent reauthentication for access changes, bulk deletion, or secret rotation. Hiding a route or omitting a link is not authorization.

Audit actor, action, target, result, and timestamp for mutations. Do not include credentials, signed URLs, or private image metadata in audit logs. Admin overview does not expose private originals or let an operator casually browse user libraries. Later moderation acts on explicitly submitted/public items.

Proposed route families, to be formalized before implementation: `GET /admin/overview`, `GET /admin/storage`, `GET /admin/jobs`, `POST /admin/jobs/{id}/retry`, `PUT /admin/intake`, and account/invite/revocation endpoints. Retries must respect idempotency and quota reservations. No arbitrary SQL, shell, URL-fetch, or object-key console.

Discovery later adds sources, reports, rights decisions, and publication withdrawal. Keep those absent from MVP, rather than disabled navigation advertising future features.

## Acceptance

An ordinary browser user, desktop user session, and device token all fail admin authorization. Expired/forged sessions and CSRF attempts fail. Pausing intake blocks new uploads while existing playback and authorized reads remain available. Usage includes derivatives and outstanding reservations; failures expose a concrete retry/recovery action. Every privileged mutation leaves an audit record.
