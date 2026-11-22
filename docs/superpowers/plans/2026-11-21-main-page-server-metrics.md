# Main page: horizontal tabs and server metrics

Date: 2026-11-21
Branch: `main-page-20261121-03`

## Task

Bring the panel navigation in line with Remnawave: horizontal tabs in the header,
the first tab «Главная» shows the host's own metrics:

- CPU: used/free,
- RAM: used/free,
- disk: free (and total),
- traffic consumed.

## Solution

- `web/src/App.svelte`: the side menu replaced with a header with a logo, horizontal
  tabs (the active one underlined) and a «Выйти» button. On narrow screens the tabs
  scroll.
- `web/src/routes/Dashboard.svelte` (route `/`): heading «Главная», a
  «Сервер» block of 4 cards (CPU, RAM, disk, traffic), below it the previous blocks under
  the heading «Трафик панели»: summary, chart, xray, online devices.
- New package `internal/host`: `Read(dataDir)` returns CPU%/cores from `/proc/stat`
  (100 ms sample), memory from `/proc/meminfo` (MemTotal/MemAvailable), disk
  via `statfs` on `DATA_DIR`. On non-Linux (local development on macOS)
  CPU/memory = 0.
- `GET /api/system` (`internal/api/system.go`) — behind the JWT middleware, body is
  `host.Stats` as JSON.
- Consumed traffic is taken from the already existing `/api/stats/summary`
  (sum of `up`+`down` over `traffic_samples`), no new counter was introduced.

## Verification

- `go test ./internal/host ./internal/api`, `go vet`.
- `npm run check` (0 errors), `npx vitest run` (22 tests).
- Browser (vite dev, 1440×900): horizontal tabs with «Главная» active,
  «Сервер» cards (CPU, RAM, disk, traffic), chart and previous blocks in place.
- On the Linux server: `curl /api/system` returns non-zero `mem_total`,
  `mem_used`, `cpu_used_percent`.

## Out of scope

- Icons in the cards (the project has no icon library).
- Auto-refresh of metrics on a timer: refresh via the «Обновить» button.
- xray process metrics (Heap, Event Loop) from the Remnawave screenshot.