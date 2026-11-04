-- +goose Up
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TYPE friendship_status AS ENUM ('pending', 'accepted', 'declined');

CREATE TYPE session_visibility AS ENUM ('friends', 'group');

CREATE TYPE degree AS ENUM ('undergraduate', 'graduate', 'phd');

CREATE TABLE schools (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE
);

CREATE TABLE majors (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name text NOT NULL UNIQUE
);

-- Searching by name.display_name isn ’ t unique
--- and isn ’ t indexed for search. Matching on it needs a trigram index (pg_trgm)
-- or a simple ILIKE limited to people at the same school.That second option is probably enough at your scale.
--
CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    apple_sub text NOT NULL UNIQUE,
    apple_refresh_token text,
    username citext NOT NULL UNIQUE,
    display_name text NOT NULL,
    school_id uuid NOT NULL REFERENCES schools (id),
    major_id uuid NOT NULL REFERENCES majors (id) ON DELETE
    SET
        NULL,
        degree degree,
        created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

CREATE INDEX auth_tokens_user_id_idx ON auth_tokens (user_id);

CREATE TABLE friendships (
    user_a uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    user_b uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    requested_by uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    status friendship_status NOT NULL DEFAULT 'pending',
    created_at timestamptz NOT NULL DEFAULT now(),
    accepted_at timestamptz,
    PRIMARY KEY (user_a, user_b),
    CHECK (requested_by IN (user_a, user_b)),
    CHECK (
        (status = 'accepted') = (accepted_at IS NOT NULL)
    )
);

CREATE INDEX friendships_user_b_idx ON friendships (user_b);

CREATE TABLE blocks (
    blocker_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    blocked_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

CREATE INDEX blocks_blocked_id_idx ON blocks (blocked_id);

CREATE TABLE groups (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX groups_owner_id_idx ON groups (owner_id);

CREATE TABLE group_members (
    group_id uuid NOT NULL REFERENCES groups (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (group_id, user_id)
);

CREATE INDEX group_members_user_id_idx ON group_members (user_id);

-- A session with NULL ended_at is active.
CREATE TABLE sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    host_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    location_text text NOT NULL,
    visibility session_visibility NOT NULL,
    group_id uuid REFERENCES groups (id) ON DELETE CASCADE,
    started_at timestamptz NOT NULL DEFAULT now(),
    planned_end_at timestamptz,
    ended_at timestamptz,
    CHECK ((visibility = 'group') = (group_id IS NOT NULL)),
    CHECK (
        ended_at IS NULL
        OR ended_at >= started_at
    )
);

-- One active session per host.
CREATE UNIQUE INDEX sessions_one_active_per_host_idx ON sessions (host_id)
WHERE
    ended_at IS NULL;

-- Auto-ending stale active sessions.
CREATE INDEX sessions_active_started_at_idx ON sessions (started_at)
WHERE
    ended_at IS NULL;

-- Pruning old ended sessions.
CREATE INDEX sessions_ended_at_idx ON sessions (ended_at)
WHERE
    ended_at IS NOT NULL;

CREATE INDEX sessions_group_id_idx ON sessions (group_id);

CREATE TABLE session_attendees (
    session_id uuid NOT NULL REFERENCES sessions (id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    PRIMARY KEY (session_id, user_id)
);

CREATE INDEX session_attendees_user_id_idx ON session_attendees (user_id);

-- APNs tokens.
CREATE TABLE device_tokens (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token text NOT NULL UNIQUE,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX device_tokens_user_id_idx ON device_tokens (user_id);

-- +goose Down
DROP TABLE device_tokens;

DROP TABLE session_attendees;

DROP TABLE sessions;

DROP TABLE group_members;

DROP TABLE groups;

DROP TABLE blocks;

DROP TABLE friendships;

DROP TABLE auth_tokens;

DROP TABLE users;

DROP TABLE majors;

DROP TABLE schools;

DROP TYPE degree;

DROP TYPE session_visibility;

DROP TYPE friendship_status;

DROP EXTENSION IF EXISTS citext;