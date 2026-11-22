# Compact server metrics cards

Branch: `main-page-compact-20261121-04` (from `dev`)
Previous step: `main-page-20261121-03` (horizontal tabs + metrics)

## Task

User: "Why such big frames for the data? Make them smaller.
So that the information fits and there is as little free space in them as possible."

The cards of the «Сервер» block were stretched to the full row width (4-column grid,
`p-4`, value `text-xl`) — at 1440px each was ~340×110 px, the content took up
a smaller part.

## Solution

- `grid sm:grid-cols-2 xl:grid-cols-4` → `flex flex-wrap gap-2`: the card is wider than
  its content but does not stretch.
- Padding `p-4` → `px-3 py-2`, radius `rounded-xl` → `rounded-lg`.
- Label and value on one line (`flex items-baseline gap-2`), value
  `text-xl` → `text-lg leading-none`; the second line (details) stayed `text-xs`.
- Layout/data unchanged, API and backend untouched.

Result at 1440px: cards 167×57, 117×57, 221×57, 196×57 instead of ~340×110.

### Follow-up: «Трафик панели»

User: "Same for «Трафик панели»". The same three summary cards
(`Отдано всего` / `Принято всего` / `Онлайн` — UI labels) moved to the same compact style:
`flex flex-wrap gap-2`, `px-3 py-2`, label and value on one line.
Result at 1440px: 196×37, 158×37, 90×37 instead of ~460×90; one row 37 px high.

## Verification

- Browser (vite dev :5199): the card row is 57 px high, each card wider than its content,
  no horizontal scroll (`scrollWidth` = 1440).
- `npm run check`, `npx vitest run`, `go test ./...`.

## Out of scope

- The «Трафик панели» cards further down were not touched — the remark was only about «Сервер».
- Mobile width < 640px still wraps one card per line (flex-wrap).