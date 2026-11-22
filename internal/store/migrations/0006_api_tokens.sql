-- Долгоживущие токены для API. Хранится только SHA-256 хэш: сам токен
-- показывается один раз при создании.
CREATE TABLE api_tokens (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  scope TEXT NOT NULL CHECK (scope IN ('full','read')),
  token_hash TEXT NOT NULL UNIQUE,
  expires_at TEXT NOT NULL DEFAULT '',
  revoked INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);