-- schema.sql mirrors the live Engram `observations` table DDL (Engram
-- design-notes #3133, live schema dump #3129) so store_test.go exercises the
-- same shape modernc.org/sqlite will see against a real ~/.engram/engram.db.
-- The `sessions` foreign key is omitted: SQLite does not validate a
-- FOREIGN KEY target at CREATE TABLE time and this fixture never enables
-- PRAGMA foreign_keys, so the referenced table is not needed for these
-- tests to be faithful.
CREATE TABLE observations (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id         TEXT,
    session_id      TEXT    NOT NULL,
    type            TEXT    NOT NULL,
    title           TEXT    NOT NULL,
    content         TEXT    NOT NULL,
    tool_name       TEXT,
    project         TEXT,
    scope           TEXT    NOT NULL DEFAULT 'project',
    topic_key       TEXT,
    normalized_hash TEXT,
    revision_count  INTEGER NOT NULL DEFAULT 1,
    duplicate_count INTEGER NOT NULL DEFAULT 1,
    last_seen_at    TEXT,
    created_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    updated_at      TEXT    NOT NULL DEFAULT (datetime('now')),
    deleted_at      TEXT,
    review_after    TEXT,
    expires_at      TEXT,
    embedding       BLOB,
    embedding_model TEXT,
    embedding_created_at TEXT,
    pinned          BOOLEAN NOT NULL DEFAULT 0
);

CREATE INDEX idx_obs_session ON observations(session_id);
CREATE INDEX idx_obs_type ON observations(type);
CREATE INDEX idx_obs_project ON observations(project);
CREATE INDEX idx_obs_created ON observations(created_at DESC);
CREATE INDEX idx_obs_scope ON observations(scope);
CREATE INDEX idx_obs_sync_id ON observations(sync_id);
CREATE INDEX idx_obs_topic ON observations(topic_key, project, scope, updated_at DESC);
CREATE INDEX idx_obs_deleted ON observations(deleted_at);
CREATE INDEX idx_obs_dedupe ON observations(normalized_hash, project, scope, type, title, created_at DESC);
