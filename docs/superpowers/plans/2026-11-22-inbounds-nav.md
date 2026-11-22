# «Inbounds» menu in the top navigation

## Task

Two dropdown menus (inbound actions and metrics) hung in the header of the
Inbounds page itself. It needs to work like Remnawave: an «Inbounds ▾» tab in the
top navigation, with items underneath.

## Solution

- `web/src/lib/inboundsIntent.svelte.ts` — shared state
  `inboundsIntent = $state({ action: 'create' | 'metrics' | null })`: the header
  sets the intent, the page applies and resets it.
- `web/src/App.svelte` — the «Inbounds» tab became a button with «▾» and a panel:
  «Инбаунды» (navigates to the list), «Добавить inbound» (intent `create`),
  «Метрики» (intent `metrics`). The active tab is underlined the same way as the
  others.
- `web/src/routes/Inbounds.svelte` — the header dropdowns were removed; the
  «Метрики» section (`Инбаунды / Онлайн / Трафик` + ↑/↓ per inbound, «Обновить»,
  «Скрыть») opens on intent or hides manually; the `create` intent opens the new
  inbound dialog right away.

## Verification

- `npm run check` — 0 errors / 12 pre-existing warnings.
- `npx vitest run` — 22/22.
- Browser: the menu opens, «Метрики» leads to `#/inbounds` and expands the
  section (locally the xray stub → «Не удалось получить метрики (502)»),
  «Добавить inbound» opens «Новый inbound».

## Out of scope

- Outbound traffic metrics and Prometheus.
- Icons in the menu items.