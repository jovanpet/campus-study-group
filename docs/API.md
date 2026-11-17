# API

HTTP/JSON API for the Campus Study Group iOS app.

- **Implemented** endpoints are on the `feat/apple-sign-in` branch (not yet merged to `main`).
- **Planned** endpoints are not built yet. The schema (`internal/store/migrations/`) already has tables for them, but there are no handlers. They are proposals; adjust them to match your tickets.

For how sign-in works internally see [auth.md](auth.md); for trying it out see [testing-auth.md](testing-auth.md).

## Conventions

- Request and response bodies are JSON. Unknown request fields are rejected with `400`. Bodies are limited to 1 MiB.
- Errors are `{"error": "message"}` with the status codes listed per endpoint.
- Protected routes need `Authorization: Bearer <token>`, where the token comes from sign-in. A missing, unknown or expired token gets `401` with `WWW-Authenticate: Bearer`. Tokens last 90 days from last use.
- IDs are UUIDs. Timestamps are RFC 3339.
- Unexpected server errors return `500 {"error": "internal error"}`; details go to the log only.

## Shared types

**User**

```json
{
  "id": "uuid",
  "username": "string | null",
  "display_name": "string | null",
  "degree": "undergraduate | graduate | phd | null",
  "school": { "id": "uuid", "name": "string" },
  "major":  { "id": "uuid", "name": "string" },
  "onboarded": false
}
```

`school`, `major`, `degree` and `username` are `null` until onboarding. `onboarded` is `true` once `username` is set.

---

## Implemented

### Health

#### `GET /health`
No auth. Returns `200` with plain text `OK`.

### Auth

#### `POST /auth/apple`
No auth. Sign in with an Apple identity token; creates the user on first sign-in.

Request:
```json
{
  "identity_token": "string (required)",
  "authorization_code": "string (required)",
  "nonce": "string (optional, the raw nonce whose SHA-256 was given to Apple)",
  "display_name": "string (optional, Apple only shares the name on first sign-in)"
}
```

Response `200`:
```json
{ "token": "string", "user": { "...User" } }
```

| Status | Meaning |
|---|---|
| 400 | Invalid JSON, or `identity_token` / `authorization_code` missing |
| 401 | Identity token failed verification, or Apple rejected the authorization code |
| 502 | Apple could not be reached |
| 503 | Apple credentials are not configured (development only) |

#### `POST /auth/dev`
**Development only**: not registered in production. Signs in without Apple.

Request: `{ "name": "string (optional, default \"default\")", "display_name": "string (optional)" }`
Response `200`: same as `POST /auth/apple`. The same `name` always signs in as the same user.

#### `POST /auth/logout`
Auth required. Deletes the token used for this request; the user's other devices stay signed in.
Response `204`, no body.

#### `DELETE /me`
Auth required. Revokes the Apple grant, then deletes the account and everything attached to it.
Response `204`, no body.

| Status | Meaning |
|---|---|
| 502 | Apple could not be reached; the account is kept, so retry |

### Profile and onboarding

#### `GET /me`
Auth required. Returns the current **User** (`200`). `401` if the account no longer exists.

#### `PUT /me/profile`
Auth required. Completes onboarding; all fields are required. Calling it again updates the profile.

Request:
```json
{
  "username": "3-20 chars of a-z, 0-9, _ and .",
  "display_name": "1-50 characters",
  "school_id": "uuid",
  "major_id": "uuid",
  "degree": "undergraduate | graduate | phd"
}
```

Response `200`: the updated **User**.

| Status | Meaning |
|---|---|
| 400 | Validation failed (message says which field) |
| 409 | Username is taken |

#### `GET /schools`
Auth required. Returns `200` with `[{ "id": "uuid", "name": "string" }]`.

#### `GET /majors`
Auth required. Returns `200` with `[{ "id": "uuid", "name": "string" }]`.

---

## Planned (not implemented)

Everything below needs auth. The shapes follow the existing tables; the `internal/friend`, `internal/session` and `internal/notify` packages are stubs.

### Users and discovery

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/users?q=` | Search by username or display name. Excludes users who blocked the caller. |
| `GET` | `/users/{id}` | Another user's public profile. |
| `PATCH` | `/me/settings` | Update `do_not_disturb` (column exists on `users`). |

### Friends

Backed by `friendships` (statuses `pending`, `accepted`, `declined`; one row per pair).

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/friends` | List accepted friends. |
| `GET` | `/friends/requests?direction=incoming\|outgoing` | List pending requests. |
| `POST` | `/friends/requests` | Send a request. Body: `{ "user_id": "uuid" }`. Notifies the addressee. |
| `POST` | `/friends/requests/{user_id}/accept` | Accept an incoming request. |
| `POST` | `/friends/requests/{user_id}/decline` | Decline an incoming request. |
| `DELETE` | `/friends/{user_id}` | Remove a friend, or cancel a pending request you sent. |

Likely errors: `404` unknown user, `409` request or friendship already exists.

### Blocks

Backed by `blocks`.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/blocks` | List users the caller has blocked. |
| `PUT` | `/blocks/{user_id}` | Block a user (also removes any friendship). Cannot block yourself (`400`). |
| `DELETE` | `/blocks/{user_id}` | Unblock. |

### Groups

Backed by `groups` and `group_members`.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/groups` | Groups the caller owns or belongs to. |
| `POST` | `/groups` | Create a group. Body: `{ "name": "string" }`. |
| `GET` | `/groups/{id}` | Group details and members. |
| `PATCH` | `/groups/{id}` | Rename (owner only). |
| `DELETE` | `/groups/{id}` | Delete (owner only). |
| `PUT` | `/groups/{id}/members/{user_id}` | Add a member (owner only). |
| `DELETE` | `/groups/{id}/members/{user_id}` | Remove a member (owner), or leave (self). |

### Study sessions

Backed by `sessions` and `session_attendees`. A session with no `ended_at` is active, and a host can have only one active session (`409` on a second). Visibility is `friends`, or `group` with a `group_id`.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/sessions` | Start a session. Body: `{ "location_text": "string", "visibility": "friends\|group", "group_id": "uuid (group only)", "planned_end_at": "timestamp (optional)" }`. Notifies the audience. |
| `GET` | `/sessions?status=active` | Active sessions visible to the caller (friends' sessions and their groups'). |
| `GET` | `/sessions/{id}` | Session details and attendees. |
| `PATCH` | `/sessions/{id}` | Update location or `planned_end_at` (host only). |
| `POST` | `/sessions/{id}/end` | End the session (host only). |
| `POST` | `/sessions/{id}/join` | Join as an attendee. |
| `DELETE` | `/sessions/{id}/join` | Leave. |

Server-side jobs, no endpoints: auto-end stale active sessions, and prune old ended ones.

### Push notifications

Backed by `device_tokens` (APNs). Delivery goes through the `notify.Notifier` interface, which only logs for now.

| Method | Path | Purpose |
|---|---|---|
| `PUT` | `/me/devices` | Register or refresh an APNs token. Body: `{ "token": "string" }`. |
| `DELETE` | `/me/devices/{token}` | Unregister (call on logout). |

Notifications are sent for friend requests, session starts and session invites, and skipped when the recipient has `do_not_disturb` on.
