# Main page: xray and online devices in the «Сервер» block

## Task

On «Главная» the xray card and the online-devices table stood as a separate block
at the bottom. They should be moved up to the server cards, and only status and
version should remain from xray (PID, uptime, restart count are not needed).

## Solution

`web/src/routes/Dashboard.svelte`:

- A compact xray card added to the «Сервер» row: «Работает»/«Остановлен»
  (green/red), version and Start/Stop/Restart buttons.
- The «Онлайн-устройства» table moved right under the server card row.
- The bottom "xray + online devices" block removed.
- Unused `fmtUptime` import removed.

## Verification

- Browser (vite dev + local panel): xray card in the metrics row, status and
  version, buttons in place; the devices table under the row; no duplicate at the bottom.
- `npm run check` — 0 errors, 12 previous warnings; `npx vitest run` — 22
  tests; `go test ./...` — unchanged (frontend-only change).

## Out of scope

- xray uptime/restarts are not shown; bring back if needed.