# Мониторинг: логи сервера и метрики Caddy

## Зачем

Панель управляет VPN и юзерами, но не показывает, что происходит на самом
сервере: почему не открывается сайт, почему Caddy не обновил сертификат, что
пишет xray, какой была нагрузка вчера. Всё это сейчас добывается только через
ssh. Нужен один пункт меню, где видно логи трёх источников и история метрик
Caddy — без Prometheus, Grafana и второго демона.

## Цели

- Пункт «Мониторинг» в шапке с двумя видами: «Логи» и «Caddy».
- «Логи»: три источника — сервер (весь journald), xray, Caddy. По 200 строк
  по умолчанию (максимум 1000), время в локальной зоне, уровень, текст.
- «Caddy»: графики RPS, ошибок в секунду, средней длительности запроса,
  in-flight, RSS процесса, здоровья апстрима; периоды 1ч/6ч/24ч/7д.
- Всё в трёх языках (ru/en/de), как остальная панель.

## Не-цели

- Prometheus, Grafana, VictoriaMetrics, Loki — никаких новых демонов.
- Алерты, уведомления, экспорт логов, поиск по логам.
- TLS-метрики: Caddy их не отдаёт (открытый issue caddyserver/caddy#1683).
- Мультисерверность: только тот хост, где запущена панель.

## Ограничения (проверено на fornex-vpn)

- Caddy v2.11.9, admin API на `127.0.0.1:2019`, `/metrics` уже отвечает 200,
  но содержит только admin/config/upstream-метрики: без глобальной опции
  `{ metrics }` HTTP-метрики запросов не собираются. Модуль есть, пересборка
  не нужна.
- Всё пишет в journald: Caddy (stderr), панель (stderr), xray (наследует
  stderr панели — в журнале это строки `_EXE=/opt/flypanel/bin/flypanel`,
  отличить их от строк панели нельзя).
- Панель работает под `flypanel` и journald читать не может: в группе
  `systemd-journal` его нет.
- Access log Caddy по умолчанию выключен.
- `ProtectSystem=strict` запрещает запись вне `/opt/flypanel`, чтению не мешает.

## Рассмотренные подходы

### Логи: как читать

- **A. Группа `systemd-journal` + файл для xray** (выбран). journald читается
  `journalctl -o json`; xray пишет в файл, потому что в журнале он неотличим
  от панели.
- B. Только файлы (Caddy `log` в файл, xray в файл, панель в файл) — логи
  панели всё равно в journald, три разных формата и три ротации.
- C. sudoers на `journalctl` — лишние права, отклонено.

Доступ выдаётся не `usermod`, а `SupplementaryGroups=systemd-journal` в unit:
декларативно, откат — удалить строку.

### Метрики: где хранить

- **A. Свой скрейп `/metrics` в sqlite** (выбран): ноль демонов, история 30
  дней, графики на существующем uPlot.
- B. VictoriaMetrics одним бинарником — нужен, только если понадобится PromQL.
- C. Prometheus + Grafana — 300–600 МБ RAM ради шести графиков, отклонено.

## Как это устроено

### Сервер

1. `flypanel.service`: `SupplementaryGroups=systemd-journal`, рестарт панели.
2. `/etc/caddy/Caddyfile`: глобальная опция `{ metrics }` и `log` в site-блоке
   (вывод по умолчанию — stderr → journald). **Правку делает оператор руками
   при деплое** (копия файла, `caddy reload`, при ошибке — откат копией), панель
   `/etc/caddy` не трогает: под `ProtectSystem=strict` и без root она физически
   не может, а привилегированный хелпер — отдельная подсистема. В
   `docs/deploy-checklist.md` добавляется шаг; UI при отсутствии HTTP-метрик
   показывает подсказку про `{ metrics }`.
3. xray.json, который генерирует панель: `"log": {"loglevel": "warning",
   "error": "<dataDir>/xray-error.log"}`. Файл обрезается при применении
   конфига, если вырос больше 5 МБ (`shortcut: своя ротация не нужна, xray
   пишет warning`).

### Бэкенд

Новый пакет `internal/monitoring`:

- `type Entry struct { Time time.Time; Source string; Level string; Message string }`
- `type Reader struct { DataDir string; run func(name string, args ...string) ([]byte, error) }`
  — `run` подменяется в тестах, в бою `exec.Command(...).Output()`.
- `Read(source string, lines int) ([]Entry, error)`:
  - `server` → `journalctl -o json -n <lines> --no-pager`;
  - `caddy` → `journalctl -u caddy -o json -n <lines> --no-pager`;
  - `xray` → последние `lines` строк `<DataDir>/xray-error.log` (нет файла →
    пустой список, не ошибка). Уровень — из префикса строки `[Warning]` /
    `[Error]` / `[Info]` регуляркой, иначе `info`.
  - journald: каждая строка — JSON, поля `__REALTIME_TIMESTAMP` (µs),
    `PRIORITY` (0–7 → `emerg|alert|crit|err|warning|notice|info|debug`),
    `MESSAGE` (строка или массив байт), `SYSLOG_IDENTIFIER` (для колонки
    источника). Строки, которые не парсятся, пропускаются.
  - `lines` ограничивается 1..1000, по умолчанию 200.

Скрейп метрик `internal/monitoring/caddy.go`:

- `Scrape(url string) (Sample, error)` — GET, парсинг текста Prometheus:
  суммирует `caddy_http_requests_total`, `caddy_http_request_errors_total`,
  `caddy_http_requests_in_flight`, `caddy_http_request_duration_seconds_sum`
  и `_count`, берёт `process_resident_memory_bytes` и минимум
  `caddy_reverse_proxy_upstreams_healthy`. Остальные строки игнорируются.
- `Sample{Requests, Errors, InFlight, DurationSum, DurationCount float64;
  RSSBytes float64; UpstreamHealthy *bool}` — `nil`, когда метрики апстримов
  нет (reverse_proxy не настроен или другая версия): в UI это «—», красный
  только при явном `0`.

Хранение: миграция `0007_caddy_samples.sql`:

```sql
CREATE TABLE caddy_samples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts TEXT NOT NULL,
  requests REAL NOT NULL,
  errors REAL NOT NULL,
  inflight REAL NOT NULL,
  duration_sum REAL NOT NULL,
  duration_count REAL NOT NULL,
  rss REAL NOT NULL,
  upstream_healthy INTEGER
);
CREATE INDEX idx_caddy_samples_ts ON caddy_samples(ts);
```

`store.CaddySampleInsert(ts, sample)`, `store.CaddySamplesSince(since)` и
`store.CaddySamplesPrune(before)`. Скрейп — отдельная горутина в `serve` раз в
15 с, адрес из нового поля конфига `CaddyAdminURL` (env `CADDY_ADMIN_URL`,
дефолт `http://127.0.0.1:2019`); каждый час удаление старше 30 дней. Ошибка
скрейпа пишется в лог **только при смене состояния** (успех→провал и обратно),
иначе недоступный Caddy залил бы журнал строкой каждые 15 секунд.

### API

- `GET /api/monitoring/logs?source=server|caddy|xray&lines=200` →
  `{"entries": [{"time": "...", "source": "caddy", "level": "info", "message": "..."}]}`.
  Неизвестный source → 400 `invalid_source`; journald недоступен (нет группы,
  нет `journalctl`) → 502 `journal_unavailable` + ключ `errors.journal_unavailable`
  в трёх локалях, как `xray_unavailable`.
- `GET /api/monitoring/caddy?range=1h|6h|24h|7d` →
  `{"points": [{"ts": "...", "rps": 0.0, "errors_per_s": 0.0, "avg_ms": 0.0,
  "inflight": 0.0, "rss": 0, "upstream_healthy": true}], "summary":
  {"requests": 0, "errors": 0, "avg_ms": 0.0, "inflight": 0.0, "rss": 0,
  "upstream_healthy": null}}`.
  Считается на бэкенде по разностям между соседними сэмплами: сброс счётчика
  (дельта < 0, Caddy перезапустился) даёт 0, а не отрицательный rate.
  Точки агрегируются по интервалу: 15 с для `1h`, 60 с для `6h`/`24h`, 15 мин
  для `7d` (не больше ~360 точек на график). Для первого интервала берётся один
  сэмпл до `since`, чтобы не потерять дельту. `summary`: requests/errors — сумма
  за период, `avg_ms` — по периоду, `inflight`/`rss`/`upstream_healthy` — из
  последнего сэмпла.
- Оба пути закрыты для read-токенов: в denylist middleware добавляется
  `/api/monitoring` (как config и users).

### Фронтенд

Один пункт меню «Мониторинг» (`nav.monitoring`) и два маршрута:
`/monitoring/logs` (по умолчанию) и `/monitoring/caddy`; переключатель — две
кнопки вверху страницы, как «Список / HWID» в Юзерах.

«Логи»: три кнопки источника, «Обновить», чекбокс «Автообновление (5 с)»,
моноширинный список строк `время · уровень · источник · текст`, цвет уровня
(ошибки — красным, warning — жёлтым). Ошибка загрузки — текст с кодом,
пустой ответ — «Записей нет».

«Caddy»: кнопки периода 1ч/6ч/24ч/7д, карточки сводки (запросов всего,
ошибок, средняя длительность, in-flight, RSS, апстрим), два графика uPlot:
RPS + ошибки/с, средняя длительность (мс) + RSS (МБ). Нет данных → «Данных
пока нет».

## Проверка

- `go test ./...`: парсер journald JSON (строка, массив байт, битый JSON),
  tail-чтение файла xray, парсер Prometheus-текста (счётчики, гистограммы,
  отсутствующие метрики), rate с отрицательной дельтой, deny для read-токена.
- vitest: английский язык на странице мониторинга, переключение источников
  (три запроса с нужным `source`), рендер точек в графике при пустом ответе не
  падает.
- Ручная: на fornex-vpn после деплоя `journalctl -u caddy` совпадает с тем,
  что показывает панель; `caddy_http_requests_total` растёт при обновлении
  страницы панели (метрика включает запрос к panel2).
- Регрессия: `make test`, `cd web && npx vitest run && npm run check`,
  `make build`.