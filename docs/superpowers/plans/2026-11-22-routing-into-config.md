# Routing → вкладка «Конфиг»: план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Убрать вкладку Routing из верхнего nav, перенеся её JSON-редактор в нижнюю секцию страницы «Конфиг».

**Architecture:** Редактор остаётся тем же (textarea + «Проверить» + «Применить» через `/api/config/routing` GET/PUT), но живёт в `Config.svelte` рядом с read-only генеральным конфигом. Обе ручки грузятся параллельно, их ошибки независимы. Старый маршрут `#/routing` ведёт на `Config`.

**Tech Stack:** Svelte 5 (руны `$state`), svelte-spa-router, vitest + @testing-library/svelte.

**Spec:** `docs/superpowers/specs/2026-11-22-routing-into-config-design.md`

## Global Constraints

- Бэкенд не менять: `/api/config/routing` GET/PUT и дефолт `{"domainStrategy":"AsIs","rules":[]}` как есть.
- Текст редактора отправляется на сервер байт-в-байт, без пере-сериализации.
- Тексты ошибок/подсказок сохранить из `Routing.svelte`: «Применено», «Некорректный JSON (422)», «Применение не удалось (422), конфиг откачен к прежнему значению», «xray недоступен (502)», warning про мгновенное применение и откат.
- Язык интерфейса — русский, стили — существующие классы (border, bg-muted/30, text-red-400 / text-emerald-400).

## Review Focus

- Ошибка загрузки routing не должна скрывать сгенерированный конфиг (секции независимы).
- Закладка `#/routing` должна открывать «Конфиг», а не пустую страницу.
- Первый заход без сохранённого routing показывает дефолтный объект, а не пустую textarea.
- Изменённый текст уходит в PUT ровно как введён.
- 422 показывается как откат, 502 — как недоступность xray; прочие коды — с числом.

---

### Task 1: Секция Routing в Config.svelte

**Files:**
- Create: `web/src/routes/Config.test.ts`
- Modify: `web/src/routes/Config.svelte`

**Interfaces:**
- Consumes: `apiFetch` из `$lib/api`, `Button` из `$lib/components/ui/button` (уже импортируется в Config.svelte); эндпоинты `GET /api/config`, `GET|PUT /api/config/routing`.
- Produces: `Config.svelte` с единственным `textarea` (`aria-label="Routing JSON"`) и кнопками «Проверить» / «Применить»; Task 2 лишь переключает маршруты и nav.

- [ ] **Step 1: Написать падающий тест**

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

- [ ] **Step 2: Убедиться, что тест падает**

Run: `cd web && npx vitest run src/routes/Config.test.ts`
Expected: FAIL — `Unable to find a label with the text of: Routing JSON`.

- [ ] **Step 3: Реализовать секцию в `Config.svelte`**

Перенести из удаляемого `Routing.svelte` состояние и функции как есть: `rText`, `rCheckMsg`, `rCheckOk`, `rApplyMsg`, `rApplyOk`, `rError`, `rCheck()`, `rApply()` (тот же разбор 422/502), но с префиксом `r`, чтобы не конфликтовать с `cfg/copied/notGenerated`. `onMount` грузит обе ручки параллельно (`Promise.all` не нужен — независимые `void`-вызовы или два `await` в одном колбэке); кнопки — существующий `Button`. Текст загрузки routing: `Не удалось загрузить routing (${res.status})`, сеть: `Сеть недоступна`.

Разметка ниже существующего блока конфига:

```svelte
<h2 class="mt-8 text-lg font-semibold">Routing (advanced JSON)</h2>
<p class="mt-2 text-sm text-muted-foreground">
  ⚠ Применение сразу изменяет работающий xray; при ошибке применения значение откатывается к прежнему.
</p>
<textarea bind:value={rText} aria-label="Routing JSON" spellcheck="false"
  class="mt-4 h-[40vh] w-full rounded-xl border bg-muted/30 p-4 font-mono text-xs"></textarea>
```

Кнопки «Проверить» (JSON.parse) и «Применить» + строка статуса — как в `Routing.svelte`. `Полный отказ от санитайзера` не трогаем.

- [ ] **Step 4: Прогнать тест**

Run: `cd web && npx vitest run src/routes/Config.test.ts`
Expected: PASS, 4/4.

- [ ] **Step 5: Commit**

```bash
git add web/src/routes/Config.svelte web/src/routes/Config.test.ts
git commit -m "feat(web): move routing editor into config page"
```

### Task 2: Убрать вкладку Routing

**Files:**
- Modify: `web/src/App.svelte`
- Delete: `web/src/routes/Routing.svelte`

**Interfaces:**
- Consumes: `Config.svelte` из Task 1 (маршрут `'/routing'` теперь указывает на него).
- Produces: nav без пункта «Routing», routes без `Routing`-компонента.

- [ ] **Step 1: Правки в `App.svelte`**

Удалить `import Routing from './routes/Routing.svelte';`, заменить в `routes` строку `'/routing': Routing,` на `'/routing': Config,` (чтобы старые закладки открывали «Конфиг»), удалить из `nav` элемент `{ href: '#/routing', path: '/routing', label: 'Routing' }`.

- [ ] **Step 2: Удалить файл и проверить, что ссылок не осталось**

```bash
git rm web/src/routes/Routing.svelte
grep -rn "Routing" web/src --include=*.svelte --include=*.ts
```

Expected: grep находит только `('/routing': Config)` и текст секции «Routing (advanced JSON)» в Config.svelte; импортов `routes/Routing.svelte` нет.

- [ ] **Step 3: Проверить сборку и всё web-тесты**

Run: `cd web && npm run check && npx vitest run`
Expected: 0 ошибок; тесты зелёные (включая `Config.test.ts`).

- [ ] **Step 4: Собрать бинарник с embedded SPA**

Run: `make build`
Expected: `bin/flypanel` собран, ошибок `go:embed` нет.

- [ ] **Step 5: Commit**

```bash
git add web/src/App.svelte
git commit -m "feat(web): drop routing tab, keep route for old links"
```

## Ручная проверка после сборки

`./bin/flypanel serve` → в шапке 6 пунктов без «Routing»; `#/config` показывает конфиг и секцию Routing; `#/routing` открывает ту же страницу «Конфиг»; правка JSON + «Применить» даёт «Применено» (локально без xray — 502).