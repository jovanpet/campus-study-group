# API

The request/response contract between the API and the iOS app (ticket #33). Update it in the same PR as any API change.

Each endpoint is marked:

- **Implemented**: built on the `feat/apple-sign-in` branch (not yet merged to `main`).
- **Planned (#n)**: not built. The shape comes from the ticket's acceptance criteria and may change when it's built. Fields the tickets don't pin down are marked *TBD*.

How sign-in works internally: [auth.md](auth.md). Trying it out: [testing-auth.md](testing-auth.md). Screens: `docs/design/` (PR #48).

## Conventions

- JSON in and out. Unknown request fields get `400`. Bodies are limited to 1 MiB.
- Errors are `{"error": "message"}`.
- Protected routes need `Authorization: Bearer <token>` from sign-in. Missing, unknown or expired tokens get `401` with `WWW-Authenticate: Bearer`. Tokens last 90 days from last use.
- IDs are UUIDs. Timestamps are RFC 3339.
- Unexpected failures return `500 {"error": "internal error"}`.
- **Planned (#35):** social endpoints (friends, sessions, groups, blocks, search) return `403` until onboarding is complete.
- **Planned (#31, #21):** rate limits on auth, write and search endpoints (details TBD).

## Shared types

**User** (the signed-in user, from sign-in and `GET /me`)

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

`username`, `display_name`, `degree`, `school` and `major` are `null` until onboarding; `onboarded` is `true` once `username` is set. Planned additions: `do_not_disturb` (#19). Ticket #35 calls the flag `needs_onboarding`; the code uses `onboarded` (the inverse), which is what the app should use.

**Person** *(planned, #21/#23/#36)*: another user, minimal public profile.

```json
{ "id": "uuid", "username": "string", "display_name": "string",
  "school": { "id": "uuid", "name": "string" }, "major": { "id": "uuid", "name": "string" } }
```

**Session** *(planned, #26/#41)*: see the endpoints below for the exact fields each one returns.

---

## Implemented

### `GET /health`
No auth. `200`, plain text `OK`.

### `POST /auth/apple`
No auth. Sign in with Apple; creates the user on first sign-in.

```json
{
  "identity_token": "string, required",
  "authorization_code": "string, required",
  "nonce": "string, optional (the raw nonce whose SHA-256 you gave Apple)",
  "display_name": "string, optional (Apple sends the name only once, #18)"
}
```

`200`: `{ "token": "string", "user": User }`

| Status | Meaning |
|---|---|
| 400 | Invalid JSON, or `identity_token` / `authorization_code` missing |
| 401 | Identity token failed verification, or Apple rejected the authorization code |
| 502 | Apple could not be reached |
| 503 | Apple credentials are not configured (development only) |

### `POST /auth/dev`
**Development only**, not registered in production. Signs in without Apple, for the simulator and curl.

`{ "name": "optional, default \"default\"", "display_name": "optional" }` → `200`, same as `/auth/apple`. The same `name` always gives the same user.

### `POST /auth/logout`
Auth. Deletes this request's token; other devices stay signed in. `204`.

### `DELETE /me`
Auth. Revokes the Apple grant, then deletes the account and all its data. `204`. `502` if Apple can't be reached; the account is kept, so retry.

### `GET /me`
Auth. `200`: **User**. `401` if the account no longer exists.

### `PUT /me/profile`
Auth. Completes onboarding (Set up profile screen). All fields required. Calling it again overwrites the profile.

```json
{ "username": "3-20 chars of a-z 0-9 _ .", "display_name": "1-50 chars",
  "school_id": "uuid", "major_id": "uuid", "degree": "undergraduate | graduate | phd" }
```

`200`: **User**. `400` validation (message names the field). `409` username taken.

> Open (#35): the Set up profile screen has no degree field, but the database requires `degree` before a username can be set. Until that's decided, the app must send a degree.

### `GET /schools`, `GET /majors`
Auth. `200`: `[{ "id": "uuid", "name": "string" }]`. The `?q=` search and the "Undeclared" major are planned (#34).

---

## Planned

### Onboarding and profile

| Endpoint | Ticket | Notes |
|---|---|---|
| `GET /usernames/{name}/available` | #35 | Live "Available" check under the username field. Response *TBD* (likely `{ "available": bool }`). |
| `PATCH /me` | #19 | Updates any of `display_name`, `username`, `school_id`, `major_id`, `degree`, `do_not_disturb`. `409` on username conflict; ids validated. Backs Edit profile and the Do not disturb switch. |
| `GET /schools?q=`, `GET /majors?q=` | #34 | Case-insensitive search. |

### Find people and profiles

**`GET /users/search?q=`** (#21). Paginated and rate limited. Case-insensitive match on username (prefix) or display name; a leading `@` searches usernames only. Excludes yourself and anyone blocked in either direction.

Returns a list of **Person** plus `friendship`: `none`, `pending_outgoing`, `pending_incoming` or `friends`. This is where the app shows (and cancels) an outgoing request, since no screen lists them.

**`GET /users/{id}`** (#47). Friend's profile.
- Always: id, display_name, username, school, major, `friendship`.
- For friends, also `friends_since` and `session` (id, location_text, started_at, planned_end_at) if you can see it, else `null`.
- Non-friends get the search fields only. `404` if either user blocked the other.

**`GET /friends/suggestions`** (#37). Not in the design; after v1 or closed.

### Friends

Mutual requests (#22).

| Endpoint | Behavior |
|---|---|
| `GET /friends` (#23) | Friends: id, display_name, username, major, sorted by name. Paginated if needed. Also feeds the Start session and New group pickers. |
| Send request | Rejects self, duplicates and blocked pairs. If they already asked you, it accepts instead. Path/body *TBD*. |
| Accept / decline | Recipient only. |
| Remove friend / cancel outgoing request | Deletes the row. |
| List incoming requests | Requester id, display_name, major (the Requests section). |
| List outgoing requests | Built but has no screen; the app reads the status from search. |

Exact paths for the request endpoints aren't in the ticket yet; settle them when #22 is built and fill them in here.

### Blocks (#36)

- `GET /blocks`: `[{ id, display_name, major }]` for the Blocked people screen.
- `POST /blocks/{userId}`: blocks. Also removes any friendship or pending request, removes each from the other's groups, and removes either from the other's session.
- `DELETE /blocks/{userId}`: unblocks only; does not restore the friendship.

### Groups (#38)

Owner only for changes. Members must be accepted friends of the owner.

- `POST /groups`: `{ "name", "member_ids": [...] }` (field names *TBD*).
- `GET /groups`: the caller's groups with members (id, display_name). Also includes an automatic same-major group, computed from friends who share your major, marked `kind: "major"`, not editable or deletable.
- `PATCH /groups/{id}`: change the name, the members, or both.
- `DELETE /groups/{id}`.

### Sessions

**`POST /sessions`** (#24, #42, #45). Start (or schedule) a session.

```json
{
  "location_text": "required free text; trimmed, whitespace collapsed",
  "started_at": "optional; default now; a future value schedules it",
  "planned_end_at": "optional; must be after started_at",
  "audience": "all_friends (default) | group | same_major",
  "group_id": "uuid, when audience is group",
  "excluded or included user ids": "TBD, depends on decision #44"
}
```

Returns the session. `409` if you already have an active session. Group audience: you must own the group. The audience is always a subset of your accepted, non-blocked friends. Friends are alerted asynchronously (#29).

> Open: audience storage (#44); how far ahead a session can be scheduled, and whether a host can have scheduled and active sessions at once (#45); max length vs `planned_end_at` (#27).

**`GET /friends/active`** (#26). The Home feed. Returns:
- `now`: active sessions from accepted friends who put you in the audience, excluding blocked users.
- `later`: scheduled sessions not started yet.
- `pending_request_count`: for the Friends button badge.

Each session: `id`, `host { id, display_name }`, `location_text`, `started_at`, `planned_end_at` (nullable), `joined` (are you an attendee). No attendee names; get them from `GET /sessions/{id}`. Empty `now` and `later` is a normal `200`. The app polls this (#7).

**`GET /me/session`** (#41). Your active session (hosted or attending) or `204`. Includes your role (host or attendee), `location_text`, `started_at`, `planned_end_at`, the audience as chosen ("All friends" or the group's name) and the attendees.

**`GET /sessions/{id}`** (#41). `host { id, display_name }`, `location_text`, `started_at`, `planned_end_at`, `people` (id, display_name, `is_host`; host first), and `joined`. `404` if you can't see it.

**`PATCH /sessions/{id}`** (#46). Host only, not ended. Edit `location_text`, `planned_end_at` (set, change or clear; must be in the future) and the audience. People removed from the audience are removed from attendees. Returns the same shape as `GET /sessions/{id}`.

**`POST /sessions/{id}/finish`** (#25). Host only; sets `ended_at`, which ends it for everyone. `409` if already finished.

**`POST /sessions/{id}/join`, `POST /sessions/{id}/leave`** (#39). You must be able to see the session and it must not have ended. Joining while in another session moves you. `409` if you're hosting your own. "I'll go" on a scheduled session uses the same endpoint. The host isn't an attendee row; responses list them first and flag them.

Scheduled sessions may also get a cancel endpoint (#45, *TBD*).

Auto-end (#27): sessions end at `planned_end_at`, or after `SESSION_MAX_DURATION` (default 4h). Reads treat overdue sessions as ended, so the app never sees a stale one.

**`GET /locations?q=`** (#40). Not in the design; after v1 or closed.

### Push notifications

- Register or unregister an APNs token (#28). Paths *TBD*; likely `PUT`/`DELETE` under `/me/devices`.
- Pushes (#29) go to the session's audience when it starts, excluding blocked users and anyone with `do_not_disturb`. Scheduled sessions: timing undecided (#45). Wording is like "Ana is studying at SciLi, 4th floor".
