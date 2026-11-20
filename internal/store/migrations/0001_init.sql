-- v1 schema. All timestamps are TEXT in RFC3339 UTC, set by the application.
CREATE TABLE admin (
    id INTEGER PRIMARY KEY CHECK(id = 1),
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    totp_secret TEXT,
    totp_enabled INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);

CREATE TABLE backup_codes (
    id INTEGER PRIMARY KEY,
    code_hash TEXT NOT NULL,
    used INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY,
    username TEXT UNIQUE NOT NULL,
    uuid TEXT NOT NULL,
    sub_token TEXT UNIQUE NOT NULL,
    expiry TEXT NOT NULL,
    quota_bytes INTEGER NOT NULL DEFAULT 0,
    quota_period TEXT NOT NULL DEFAULT 'none' CHECK(quota_period IN ('none','day','week','month')),
    quota_reset_at TEXT NOT NULL,
    device_limit INTEGER NOT NULL DEFAULT 0,
    disabled INTEGER NOT NULL DEFAULT 0,
    disabled_reason TEXT,
    created_at TEXT NOT NULL
);

CREATE TABLE inbounds (
    id INTEGER PRIMARY KEY,
    tag TEXT UNIQUE NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    config_json TEXT NOT NULL,
    created_at TEXT NOT NULL
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE traffic_samples (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL,
    ts TEXT NOT NULL,
    up_delta INTEGER NOT NULL,
    down_delta INTEGER NOT NULL
);
CREATE INDEX idx_traffic_samples_username_ts ON traffic_samples(username, ts);

CREATE TABLE devices (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL,
    ip TEXT NOT NULL,
    first_seen TEXT NOT NULL,
    last_seen TEXT NOT NULL,
    UNIQUE(username, ip)
);

CREATE TABLE refresh_tokens (
    id INTEGER PRIMARY KEY,
    token_hash TEXT UNIQUE NOT NULL,
    expires_at TEXT NOT NULL,
    revoked INTEGER NOT NULL DEFAULT 0
);
