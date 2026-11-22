# Calendar in the «Действует до» field

## Task

In the user dialog the expiry date was entered with a `datetime-local` field: the
calendar opened in Chrome, but in the dark theme the field looked like a plain
input and the date and time were glued into one line.

## Solution

`web/src/lib/components/UserDialog.svelte`:

- the state is split into `expiryDate` (`YYYY-MM-DD`) and `expiryTime` (`HH:MM`),
  both taken from the initial value (`toLocalInput(...)`), +30 days by default;
- in the «Настройки доступа» section, instead of a single field there is
  `Input type="date"` (the native Chrome calendar) and `Input type="time"` next
  to it;
- on submit `new Date(`${expiryDate}T${expiryTime || '00:00'}`)` is assembled and
  sent to the API as ISO.

## Verification

- `npm run check` — 0 errors (12 pre-existing warnings), `npx vitest run` — 22/22.
- Browser: the «Создать пользователя» dialog shows the date `2026-12-21` and the
  time `21:13`; creating the user stores `expiry = 2026-12-21T18:13:00Z`, that
  is 21:13 local time (UTC+3) — date and time are not lost.

## Out of scope

- A custom calendar on bits-ui (the native `type="date"` gives the same dropdown
  calendar without a new dependency).
## Continuation: a custom calendar popover

The native `date`/`time` were replaced with a Remnawave-style popover: presets
«+1 месяц», «+3 месяца», «+1 год», «до 2099 года» on the left, a calendar with
month switching, a 6×7 grid starting on Monday (weekends in red, the selected day
highlighted) on the right, and the time plus a confirm button at the bottom.

The popover opens upwards (`bottom-full`), and while it is open the dialog panel
is `overflow-visible`, otherwise scrolling clipped the calendar.

Verification: `svelte-check` 0 errors / 12 warnings, vitest 22/22, a screenshot of
the popover — presets, grid, day selection and confirmation fully visible.