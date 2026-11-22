# Fix: «Изменить» in Inbounds does not open the dialog

**Branch:** `inbounds-edit-dialog-20261121-01` from `dev`
**Spec:** `docs/superpowers/specs/2026-11-20-fly-vpn-panel-design.md`
**Bug report:** "In the Inbounds tab, the button «Изменить» for an existing inbound. Nothing happens when it is clicked" (21.11.2026).

## Symptom

The «Изменить» button in the Inbounds table gives no visible reaction: the dialog does not appear.

## Root cause

`InboundForm.build()` called `structuredClone(inbound.config)`. The `inbounds` row comes from `apiFetch('/api/inbounds')` and is put into `$state`, so `inbound.config` is a Svelte 5 deep proxy. `structuredClone` on a proxy throws `DataCloneError`, the exception aborts the component's first render → the dialog never mounts. State and the button handler are fine; the create path (`inbound === null`) is unaffected.

Confirmed by a live repro in the browser: `DataCloneError: Failed to execute 'structuredClone' on 'Window': #<Object> could not be cloned.` at `InboundForm.svelte:78`.

## Change

- `web/src/lib/components/InboundForm.svelte`: `const cfg = structuredClone(cfg0)` → `const cfg = $state.snapshot(cfg0)`, one line + a "why" comment.
- `web/src/lib/components/InboundForm.test.ts` (new): render with a proxy config (`new Proxy`) — without the fix it fails with `DataCloneError`, with the fix it passes.

## Verification

- [x] Live browser (vite dev + Playwright): the «Изменить inbound» dialog opens, fields prefilled (8443 / `/xhttp` / `www.microsoft.com`).
- [x] Saving: «Сохранить» → PATCH `/api/inbounds/1` → `streamSettings.xhttpSettings.path` changed in the DB.
- [x] «Добавить» → «Новый inbound» opens (no regression).
- [x] `npm run check` — 0 errors; `npx vitest run` — 8 files / 22 tests; `go test ./...` — ok.
- [x] The new test fails on the old code and passes on the new one (verified by a temporary revert).
- [x] `make cross` → deploy to VPS, service active, `/sub/<token>` 200, the panel serves the new bundle.

## Out of scope

- The bug "the subscription is not added in Hiddify/Incy" is not a code defect: the VPS IP is unreachable from the home provider's network (ICMP+TCP 80/443/8443, 0 packets on the server per tcpdump). Separate infra task, does not touch code.