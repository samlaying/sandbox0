-- +goose Up
-- Session Store checkpoints bind durable managed-agent sessions to volume snapshots.

CREATE TABLE IF NOT EXISTS session_checkpoints (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL,
    team_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    volume_id TEXT NOT NULL REFERENCES sandbox_volumes(id) ON DELETE CASCADE,
    snapshot_id TEXT NOT NULL REFERENCES sandbox_volume_snapshots(id) ON DELETE CASCADE,
    parent_checkpoint_id TEXT REFERENCES session_checkpoints(id) ON DELETE SET NULL,
    event_seq BIGINT NOT NULL DEFAULT 0,

    label TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'manual',
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    created_from_event_id TEXT,
    context_recipe JSONB,
    metadata JSONB,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_session_checkpoints_session_id ON session_checkpoints(session_id);
CREATE INDEX IF NOT EXISTS idx_session_checkpoints_team_session ON session_checkpoints(team_id, session_id);
CREATE INDEX IF NOT EXISTS idx_session_checkpoints_snapshot_id ON session_checkpoints(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_session_checkpoints_parent_id ON session_checkpoints(parent_checkpoint_id);
CREATE INDEX IF NOT EXISTS idx_session_checkpoints_event_seq ON session_checkpoints(team_id, session_id, event_seq);
CREATE INDEX IF NOT EXISTS idx_session_checkpoints_smartest
    ON session_checkpoints(team_id, session_id, score DESC, created_at DESC)
    WHERE kind = 'smartest';

-- +goose Down
DROP TABLE IF EXISTS session_checkpoints;
