# Remove «Онлайн-устройства» from the main page

## Task

The «Онлайн-устройства» block on «Главная» is not needed: the user asked to remove it
from the main screen.

## Solution

- `web/src/routes/Dashboard.svelte`: the table block removed, the `Device` type, the
  `devices` state, the `fmtDate` helper and the `/api/stats/online` request removed
  from `loadRest`.
- Backend unchanged: `GET /api/stats/online` remains available for the API.

## Verification

- `svelte-check` — 0 errors (12 previous warnings).
- Browser: only «Сервер» and «Трафик панели» remain on «Главная», there is no
  «Онлайн-устройства» text.
- `vitest`, `go test ./...` — unchanged.

## Out of scope

- Removing the `/api/stats/online` endpoint from the API.