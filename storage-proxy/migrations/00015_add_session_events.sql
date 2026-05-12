-- +goose Up
-- Immutable Session event log for harness context reconstruction and wake recovery.

CREATE TABLE IF NOT EXISTS session_events (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    team_id TEXT NOT NULL,
    seq BIGINT NOT NULL,

    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    metadata JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (team_id, session_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_session_events_session_seq
    ON session_events(team_id, session_id, seq);
CREATE INDEX IF NOT EXISTS idx_session_events_type
    ON session_events(team_id, session_id, event_type);

CREATE TABLE IF NOT EXISTS session_stage_entries (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    team_id TEXT NOT NULL,
    selector JSONB NOT NULL,
    note TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_session_stage_entries_session
    ON session_stage_entries(team_id, session_id, created_at);

CREATE TABLE IF NOT EXISTS session_refs (
    session_id TEXT NOT NULL,
    team_id TEXT NOT NULL,
    ref_type TEXT NOT NULL,
    name TEXT NOT NULL,
    checkpoint_id TEXT NOT NULL REFERENCES session_checkpoints(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (team_id, session_id, ref_type, name)
);

CREATE INDEX IF NOT EXISTS idx_session_refs_checkpoint_id
    ON session_refs(checkpoint_id);

DROP TRIGGER IF EXISTS update_session_refs_updated_at ON session_refs;
CREATE TRIGGER update_session_refs_updated_at
    BEFORE UPDATE ON session_refs
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TABLE IF NOT EXISTS session_harness_cursors (
    session_id TEXT NOT NULL,
    team_id TEXT NOT NULL,
    harness_id TEXT NOT NULL,

    last_seen_seq BIGINT NOT NULL DEFAULT 0,
    state JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (team_id, session_id, harness_id)
);

DROP TRIGGER IF EXISTS update_session_harness_cursors_updated_at ON session_harness_cursors;
CREATE TRIGGER update_session_harness_cursors_updated_at
    BEFORE UPDATE ON session_harness_cursors
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- +goose Down
DROP TRIGGER IF EXISTS update_session_harness_cursors_updated_at ON session_harness_cursors;
DROP TRIGGER IF EXISTS update_session_refs_updated_at ON session_refs;
DROP TABLE IF EXISTS session_harness_cursors;
DROP TABLE IF EXISTS session_refs;
DROP TABLE IF EXISTS session_stage_entries;
DROP TABLE IF EXISTS session_events;
