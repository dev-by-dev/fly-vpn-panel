-- Сэмплы метрик Caddy, снимаемые панелью раз в 15 секунд. Счётчики
-- хранятся как есть (растут), разницы считает API при агрегации.
CREATE TABLE caddy_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT NOT NULL,
  requests REAL NOT NULL,
  errors REAL NOT NULL,
  in_flight REAL NOT NULL,
  duration_sum REAL NOT NULL,
  duration_count REAL NOT NULL,
  rss REAL NOT NULL,
  upstream_healthy INTEGER
);
CREATE INDEX idx_caddy_samples_ts ON caddy_samples(ts);
