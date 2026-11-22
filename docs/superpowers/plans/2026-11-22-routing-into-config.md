# Routing → «Конфиг» tab: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the Routing tab from the top nav, moving its JSON editor into the bottom section of the «Конфиг» page.

**Architecture:** The editor stays the same (textarea + «Проверить» + «Применить» via `/api/config/routing` GET/PUT), but lives in `Config.svelte` next to the read-only general config. Both endpoints load in parallel, their errors are independent. The old route `#/routing` leads to `Config`.

**Tech Stack:** Svelte 5 (runes `$state`), svelte-spa-router, vitest + @testing-library/svelte.

**Spec:** `docs/superpowers/specs/2026-11-22-routing-into-config-design.md`

## Global Constraints

- Do not change the backend: `/api/config/routing` GET/PUT and the default `{"domainStrategy":"AsIs","rules":[]}` as they are.
- The editor text is sent to the server byte-for-byte, without re-serialisation.
- Keep the error/hint texts from `Routing.svelte`: «Применено», «Некорректный JSON (422)», «Применение не удалось (422), конфиг откачен к прежнему значению», «xray недоступен (502)», the warning about instant application and rollback.
- The interface language is Russian, the styles are the existing classes (border, bg-muted/30, text-red-400 / text-emerald-400).

## Review Focus

- A routing load error must not hide the generated config (the sections are independent).
- The `#/routing` bookmark must open «Конфиг», not an empty page.
- The first visit without a saved routing shows the default object, not an empty textarea.
- The changed text goes into the PUT exactly as entered.
- 422 is shown as a rollback, 502 as xray being unreachable; other codes with the number.

---

### Task 1: Routing section in Config.svelte

**Files:**
- Create: `web/src/routes/Config.test.ts`
- Modify: `web/src/routes/Config.svelte`

**Interfaces:**
- Consumes: `apiFetch` from `$lib/api`, `Button` from `$lib/components/ui/button` (already imported in Config.svelte); endpoints `GET /api/config`, `GET|PUT /api/config/routing`.
- Produces: `Config.svelte` with a single `textarea` (`aria-label="Routing JSON"`) and «Проверить» / «Применить» buttons; Task 2 only switches routes and nav.

- [ ] **Step 1: Write the failing test**

```ts
// web/src/routes/Config.test.ts
import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import Config from './Config.svelte';

const routing = '{"domainStrategy":"AsIs","rules":[]}';

function json(body: string, status = 200) {
  return Promise.resolve(new Response(body, { status }));
}

let putBody = '';
let routingStatus = 200;

function mockFetch() {
  vi.stubGlobal('fetch', vi.fn((input: string | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    if (url === '/api/config') return json('{"inbounds":[]}');
    if (url === '/api/config/routing' && method === 'GET') {
      return routingStatus === 200 ? json(routing) : json('{"error":"internal"}', routingStatus);
    }
    if (url === '/api/config/routing' && method === 'PUT') {
      putBody = String(init?.body ?? '');
      return json('', 200);
    }
    return json('{}', 404);
  }));
}

beforeEach(() => { putBody = ''; routingStatus = 200; mockFetch(); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

test('показывает дефолтный routing в textarea', async () => {
  render(Config);
  const ta = await screen.findByLabelText('Routing JSON');
  await waitFor(() => expect((ta as HTMLTextAreaElement).value).toContain('domainStrategy'));
});

test('Применить отправляет ровно введённый текст', async () => {
  render(Config);
  const ta = (await screen.findByLabelText('Routing JSON')) as HTMLTextAreaElement;
  await waitFor(() => expect(ta.value).toContain('domainStrategy'));
  ta.value = '{"domainStrategy":"IPIfNonMatch","rules":[]}';
  await fireEvent.input(ta);
  await fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
  await waitFor(() => expect(putBody).toBe('{"domainStrategy":"IPIfNonMatch","rules":[]}'));
  await screen.findByText('Применено');
});

test('422 объясняет откат', async () => {
  vi.stubGlobal('fetch', vi.fn((input: string | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/config') return json('{"inbounds":[]}');
    if (init?.method === 'PUT') return json('{"error":"invalid_json"}', 422);
    return json(routing);
  }));
  render(Config);
  await screen.findByLabelText('Routing JSON');
  await fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
  await screen.findByText('Некорректный JSON (422)');
});

test('ошибка загрузки routing не скрывает конфиг', async () => {
  routingStatus = 500;
  render(Config);
  await screen.findByText('Не удалось загрузить routing (500)');
  expect(screen.getByText(/inbounds/)).toBeTruthy();
});
```

- [ ] **Step 2: Confirm the test fails**

Run: `cd web && npx vitest run src/routes/Config.test.ts`
Expected: FAIL — `Unable to find a label with the text of: Routing JSON`.

- [ ] **Step 3: Implement the section in `Config.svelte`**

Move the state and functions over from the `Routing.svelte` being deleted as they are: `rText`, `rCheckMsg`, `rCheckOk`, `rApplyMsg`, `rApplyOk`, `rError`, `rCheck()`, `rApply()` (the same 422/502 handling), but with the `r` prefix so they don't clash with `cfg/copied/notGenerated`. `onMount` loads both endpoints in parallel (no need for `Promise.all` — independent `void` calls or two `await`s in one callback); buttons are the existing `Button`. The routing load text: `Не удалось загрузить routing (${res.status})`, network: `Сеть недоступна`.

Markup below the existing config block:

```svelte
<h2 class="mt-8 text-lg font-semibold">Routing (advanced JSON)</h2>
<p class="mt-2 text-sm text-muted-foreground">
  `⚠ Применение сразу изменяет работающий xray; при ошибке применения значение откатывается к прежнему.` (UI warning string)
</p>
<textarea bind:value={rText} aria-label="Routing JSON" spellcheck="false"
  class="mt-4 h-[40vh] w-full rounded-xl border bg-muted/30 p-4 font-mono text-xs"></textarea>
```

The «Проверить» (JSON.parse) and «Применить» buttons + status line — as in `Routing.svelte`. `Полный отказ от санитайзера` is left untouched.

- [ ] **Step 4: Run the test**

Run: `cd web && npx vitest run src/routes/Config.test.ts`
Expected: PASS, 4/4.

- [ ] **Step 5: Commit**

```bash
git add web/src/routes/Config.svelte web/src/routes/Config.test.ts
git commit -m "feat(web): move routing editor into config page"
```

### Task 2: Remove the Routing tab

**Files:**
- Modify: `web/src/App.svelte`
- Delete: `web/src/routes/Routing.svelte`

**Interfaces:**
- Consumes: `Config.svelte` from Task 1 (the `'/routing'` route now points at it).
- Produces: nav without the «Routing» item, routes without the `Routing` component.

- [ ] **Step 1: Edits in `App.svelte`**

Delete `import Routing from './routes/Routing.svelte';`, replace the line `'/routing': Routing,` in `routes` with `'/routing': Config,` (so old bookmarks open «Конфиг»), remove the element `{ href: '#/routing', path: '/routing', label: 'Routing' }` from `nav`.

- [ ] **Step 2: Delete the file and check no references remain**

```bash
git rm web/src/routes/Routing.svelte
grep -rn "Routing" web/src --include=*.svelte --include=*.ts
```

Expected: grep finds only `('/routing': Config)` and the «Routing (advanced JSON)» section text in Config.svelte; no imports of `routes/Routing.svelte`.

- [ ] **Step 3: Check the build and all web tests**

Run: `cd web && npm run check && npx vitest run`
Expected: 0 errors; tests green (including `Config.test.ts`).

- [ ] **Step 4: Build the binary with the embedded SPA**

Run: `make build`
Expected: `bin/flypanel` built, no `go:embed` errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/App.svelte
git commit -m "feat(web): drop routing tab, keep route for old links"
```

## Manual check after the build

`./bin/flypanel serve` → 6 items in the header without «Routing»; `#/config` shows the config and the Routing section; `#/routing` opens the same «Конфиг» page; editing the JSON + «Применить» gives «Применено» (locally without xray — 502).