# «Пользователи» cards on «Главная»

## Task

The user rejected the online-device cards: «Главная» needs overall
user statistics as a row of cards — like in Remnawave:
an icon in a colored square, a label, a large number.

## Solution

- The «Онлайн-устройства» block removed from «Главная» (together with `/api/stats/online`,
  the Device type and `fmtDate`); the backend route remains.
- Counters are computed on the client from `GET /api/users` (the list is no longer cut
  to 100 — it was cut only for the chart select):
  - Expired — `expiry <= now`;
  - Limited — `quota_bytes > 0 && used_bytes >= quota_bytes` and not expired;
  - Disabled — `disabled` and not expired/limited;
  - Active — the rest.
- Card row `flex flex-wrap gap-2` with icons: «Всего» (blue), Active
  (green, pulse), Expired (red, clock), Limited (amber, hourglass),
  Disabled (gray, ban).

## Verification

- `svelte-check` — 0 errors, 12 previous warnings.
- Local panel: «Всего 1 / Active 1 / Expired 0 / Limited 0 / Disabled 0».
- The chart select still shows users.

## Out of scope

- Server-side aggregation of counters in a separate endpoint.

## Addendum: icons in the «Сервер» cards

The CPU, RAM, disk, traffic and xray cards got the same grid as
«Пользователи»: a colored icon in a square, label on the right, a large value and
a clarifying line. Colors: CPU — cyan, RAM — violet, disk — amber,
traffic — blue, xray — green/red by state.

## Addendum: xray status as a dot

The «Работает/Остановлен» text in the xray card was replaced by a colored dot
(green/red/gray), the text stayed in `title`/`aria-label`. Version and buttons
unchanged.

## Addendum: xray version as numbers

`xray version` prints "Xray 26.11.9 (Xray, Penetrates Everything.) 676eaf6 …";
the whole line ended up in the card. Now `parseVersion` extracts the number
(`\d+\.\d+\.\d+`) from the first line, fallback — the line as is (test stub).

Verification: `TestParseVersion` (real xray line, test stub, a line without a
number) and `TestManagerStartStop` expects «25.11.20».