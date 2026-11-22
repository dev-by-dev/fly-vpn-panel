# Inbounds: two dropdown menus (actions and metrics)

## Task

On the Inbounds page the actions were spread across the table row. Two dropdown
menus are needed: one for adding/configuring/deleting inbounds, the other for
their metrics (like "Metrics Overview" in Remnawave).

## Solution

Backend:

- `xrayclient.InboundTraffic()` — a single `QueryStats` without Pattern, parsing
  names `inbound>>>{tag}>>>traffic>>>uplink|downlink` into
  `[]InboundTraffic{Tag, Up, Down}`. The counters are already enabled in the
  generated config (`stats` + `policy` with
  `statsInboundUplink/Downlink`), no new xray settings are needed.
- `GET /api/inbounds/metrics` → `{"inbounds": [...], "online_count": N}`;
  502 `xray_unavailable` when gRPC is unreachable.

Frontend (`web/src/routes/Inbounds.svelte`):

- two menu buttons in the header, «Действия ▾» and «Метрики ▾»;
- «Действия»: «Добавить inbound», and further down, per inbound, «Настроить» and
  «Удалить»;
- «Метрики»: «Инбаунды / Онлайн / Трафик» cards and a tag list with ↑UP / ↓DOWN,
  a «Обновить» button, «Загрузка…» states and an error.

## Verification

- `go build ./...`, `go test ./internal/...` — green.
- `npm run check` — 0 errors (12 pre-existing warnings), `vitest` 22/22.
- Locally: the «Действия» menu shows «Добавить inbound» + the inbound row;
  «Метрики» returns 502 `xray_unavailable` (xray stub without gRPC) and shows the
  error — the route is mounted.
- In production: cards and counters for `vless-xhttp` and `vless-tcp`.

## Out of scope

- Outbound metrics (DIRECT) and a Prometheus endpoint — the panel has no outbound
  logic.
- Metrics history: xray counters are cumulative since start. Live values
  accumulate across process restarts.