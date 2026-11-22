# Smooth chart and 24-hour labels

Branch: `chart-smooth-20261121-05` (from `dev`)

## Task

User: "Now the chart itself. It should be wavy, not sharp-angled.
Time in 24-hour format."

## Solution

`web/src/lib/components/TrafficChart.svelte`:

- Series lines: `paths: uPlot.paths.spline()` for «Отдача» and «Загрузка» —
  monotone cubic from uPlot, smooths without overshooting beyond the data
  (plain Catmull-Rom can dip below zero and draw "humps" above a peak).
  The `uPlot.paths` static is not described in the `export = uPlot` types, so it is
  taken through a local cast.
- X axis: custom formatter instead of the default (`8pm`, `11/20/26`):
  - range ≤ 24 h → `HH:MM` (`toLocaleTimeString('ru-RU', {hour12:false})`);
  - range > 24 h → `DD.MM HH:MM`.

## Verification

- Browser (vite dev :5199, 1440×900): day range — smooth waves,
  labels `20:00 … 19:00`, no `pm/am`; week range — `HH:MM` labels
  (local data covers only one day; on real 7 days it will be `DD.MM HH:MM`).
- `npm run check` — 0 errors; `npx vitest run` — 22 tests.

## Out of scope

- Line style/thickness, data points, tooltip — unchanged.
- The "24 h" threshold right at the boundary (span == 86400) shows time only —
  which is what a daily range needs.