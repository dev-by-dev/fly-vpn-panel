# Online devices on «Главная» as cards

## Task

After the online-devices table was removed, the user asked to keep them on the
main screen but present them as a row of compact cards with an icon, a label and
a value (in the style of Remnawave statistic cards).

## Solution

In `web/src/routes/Dashboard.svelte`:

- brought back the `Device` type, the `devices` state, `fmtDate()` and the
  `/api/stats/online` request in `loadRest()`;
- a «Онлайн-устройства» block added under the server metrics row:
  `flex flex-wrap gap-2`, card `rounded-lg border bg-card px-3 py-2`,
  on the left a device icon in a tinted square `bg-primary/10 text-primary`,
  on the right — IP in small mono, username large, last activity time;
- empty state — «Никого нет».

Backend unchanged: `/api/stats/online` returns devices seen during two
poller intervals.

## Verification

- Browser (vite dev + local panel): the device card renders
  (IP, user, time), an empty list shows «Никого нет».
- `npm run check`, `npx vitest run`, `go test ./...`.

## Out of scope

- Grouping devices by user and actions on a device.