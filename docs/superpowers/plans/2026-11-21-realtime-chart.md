# Real-time mode for the traffic chart

## Task

On «Главная» the traffic chart shows history for a day/week/month. A
«В реальном времени» toggle is needed: while it is on, the chart shows the last 15 minutes
of raw samples, refreshes every 5 seconds, and next to it the current inbound and
outbound speeds are shown (like in Remnawave: «Входящий 4.2 Мбит/с»).

## Solution

Backend:

- `store.SamplesSeries(from)` — raw `traffic_samples`, grouped by
  timestamp and summed over users (a sample is taken every `POLL_INTERVAL_SEC`).
- `GET /api/stats/live?minutes=15` — window 1…120 minutes, response
  `{"interval_sec": N, "buckets": [{"TS","Up","Down"}]}`.

Frontend:

- `fmtRate(bps)` — bits/s, Кбит/с, Мбит/с, Гбит/с.
- `TrafficChart` accepts `rate`: axes and tooltips are formatted as speed.
- On «Главная» a toggle in the chart card header: enables polling `/api/stats/live`
  every 5 s and recomputing bytes/interval → bit/s. The range buttons and the user select
  turn the mode off. Next to it — the current «↓ Входящий / ↑ Исходящий».

## Verification

- `go test ./...` — including `TestSamplesSeries` (summing over users and cutting off
  old samples).
- `npm run check`, `npx vitest run` — no new errors.
- Browser (vite dev + local panel): the toggle enables the mode, axis labels
  «Мбит/с», current speeds match the last sample, range 15 minutes.
- `/api/stats/live?minutes=0` → 400.

## Out of scope

- A dedicated stream endpoint (SSE/WebSocket) — polling every 5 s is sufficient with
  a sample every 30 s.
- More frequent sampling of xray counters.