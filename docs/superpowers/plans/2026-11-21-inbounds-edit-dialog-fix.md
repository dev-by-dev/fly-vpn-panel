# Fix: «Изменить» в Inbounds не открывает диалог

**Ветка:** `inbounds-edit-dialog-20261121-01` от `dev`
**Spec:** `docs/superpowers/specs/2026-11-20-fly-vpn-panel-design.md`
**Баг-репорт:** «Во вкладке Inbounds кнопка Изменить существующий inbound. Ничего не происходит при ее нажатии» (21.11.2026).

## Симптом

Кнопка «Изменить» в таблице Inbounds не даёт видимой реакции: диалог не появляется.

## Root cause

`InboundForm.build()` вызывал `structuredClone(inbound.config)`. Строка `inbounds` получена из `apiFetch('/api/inbounds')` и положена в `$state`, то есть `inbound.config` — deep-proxy Svelte 5. `structuredClone` на прокси бросает `DataCloneError`, исключение обрывает первый рендер компонента → диалог не монтируется. State и обработчик кнопки исправны; путь создания (`inbound === null`) не затронут.

Подтверждено живым репро в браузере: `DataCloneError: Failed to execute 'structuredClone' on 'Window': #<Object> could not be cloned.` на `InboundForm.svelte:78`.

## Изменение

- `web/src/lib/components/InboundForm.svelte`: `const cfg = structuredClone(cfg0)` → `const cfg = $state.snapshot(cfg0)`, одна строка + комментарий «почему».
- `web/src/lib/components/InboundForm.test.ts` (новый): рендер с конфигом-прокси (`new Proxy`) — без фикса падает `DataCloneError`, с фиксом проходит.

## Верификация

- [x] Живой браузер (vite dev + Playwright): диалог «Изменить inbound» открывается, поля предзаполнены (8443 / `/xhttp` / `www.microsoft.com`).
- [x] Сохранение: «Сохранить» → PATCH `/api/inbounds/1` → в БД изменился `streamSettings.xhttpSettings.path`.
- [x] «Добавить» → «Новый inbound» открывается (регрессии нет).
- [x] `npm run check` — 0 errors; `npx vitest run` — 8 файлов / 22 теста; `go test ./...` — ok.
- [x] Новый тест падает на старом коде и проходит на новом (проверено временным откатом).
- [x] `make cross` → деплой на VPS, сервис active, `/sub/<token>` 200, панель отдаёт новый бандл.

## Вне скоупа

- Баг «подписка не добавляется в Hiddify/Incy» — не дефект кода: IP VPS недостижим из сети домашнего провайдера (ICMP+TCP 80/443/8443, 0 пакетов на сервере per tcpdump). Отдельная инфра-задача, кода не касается.