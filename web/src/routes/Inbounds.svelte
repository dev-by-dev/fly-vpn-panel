<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import InboundForm from '$lib/components/InboundForm.svelte';
  import { inboundsIntent } from '$lib/inboundsIntent.svelte';

  type InboundRow = { id: number; tag: string; enabled: boolean; config: Record<string, any> };

  let inbounds = $state<InboundRow[]>([]);
  let error = $state('');
  let editing = $state<InboundRow | 'create' | null>(null);
  // Панель метрик открывается из меню вкладки «Inbounds» в шапке.
  let showMetrics = $state(false);
  let metrics = $state<{ inbounds: { Tag: string; Up: number; Down: number }[]; online_count: number } | null>(null);
  let metricsErr = $state('');

  const applyWarn = 'Сохранено в БД, но применить не удалось — xray работает со старым конфигом, повторите позже.';

  function cfgPort(inb: InboundRow): string {
    return String(inb.config.port ?? '?');
  }

  function cfgPath(inb: InboundRow): string {
    return inb.config.streamSettings?.xhttpSettings?.path ?? '?';
  }

  async function load() {
    try {
      const res = await apiFetch('/api/inbounds');
      if (!res.ok) {
        error = `Не удалось загрузить (${res.status})`;
        return;
      }
      inbounds = (await res.json()) as InboundRow[];
      error = '';
    } catch {
      error = 'Сеть недоступна';
    }
  }

  onMount(load);

  function fmtBytes(n: number): string {
    if (n <= 0) return '0 Б';
    const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
    const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    return `${(n / 1024 ** i).toFixed(1).replace(/\.0$/, '')} ${units[i]}`;
  }

  async function loadMetrics() {
    try {
      const res = await apiFetch('/api/inbounds/metrics');
      if (!res.ok) {
        metricsErr = `Не удалось получить метрики (${res.status})`;
        return;
      }
      metrics = (await res.json()) as typeof metrics;
      metricsErr = '';
    } catch {
      metricsErr = 'Сеть недоступна';
    }
  }

  // Намерение из шапки: одноразовое действие при переходе на страницу.
  $effect(() => {
    const action = inboundsIntent.action;
    if (!action) return;
    inboundsIntent.action = null;
    if (action === 'create') {
      editing = 'create';
    } else {
      showMetrics = true;
      void loadMetrics();
    }
  });

  async function closeAndReload() {
    editing = null;
    await load();
  }

  async function toggle(inb: InboundRow, enabled: boolean) {
    const res = await apiFetch(`/api/inbounds/${inb.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ enabled }),
    });
    if (res.status === 502) alert(applyWarn);
    await load();
  }

  async function regenKeys(inb: InboundRow) {
    if (!confirm(`Перегенерировать Reality-ключи для ${inb.tag}? Клиентам нужен новый publicKey.`)) return;
    const res = await apiFetch(`/api/inbounds/${inb.id}/keys`, { method: 'POST' });
    if (res.status === 502) {
      alert(applyWarn);
    } else if (res.ok) {
      const data = (await res.json()) as { public_key: string };
      alert(`Новый publicKey: ${data.public_key}`);
    }
    await load();
  }

  async function remove(inb: InboundRow) {
    if (!confirm(`Удалить ${inb.tag}?`)) return;
    const res = await apiFetch(`/api/inbounds/${inb.id}`, { method: 'DELETE' });
    if (res.status === 502) alert(applyWarn);
    await load();
  }
</script>

<div class="flex items-center justify-between">
  <h1 class="text-2xl font-semibold">Inbounds</h1>
</div>

{#if showMetrics}
  <div class="mt-4 max-w-2xl rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-4">
    <div class="flex items-center justify-between">
      <div class="flex items-center gap-2">
        <span class="grid h-8 w-8 place-items-center rounded-lg bg-emerald-500/15 text-emerald-300">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-4"><path d="M12 3v12m0 0 4-4m-4 4-4-4M4 19h16" /></svg>
        </span>
        <h2 class="font-medium">Inbound Traffic</h2>
      </div>
      <div class="flex gap-1">
        <Button size="xs" variant="outline" onclick={loadMetrics}>Обновить</Button>
        <Button size="xs" variant="ghost" onclick={() => (showMetrics = false)}>Скрыть</Button>
      </div>
    </div>

    {#if metrics}
      <div class="mt-3 grid grid-cols-3 gap-2">
        <div class="flex items-center gap-2 rounded-lg border bg-card/60 px-3 py-2">
          <span class="grid h-7 w-7 place-items-center rounded-md bg-sky-500/15 text-sky-400">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-3.5"><path d="m12 2 9 5-9 5-9-5 9-5Z" /><path d="m3 12 9 5 9-5" /></svg>
          </span>
          <div>
            <div class="text-xs text-muted-foreground">Инбаунды</div>
            <div class="text-lg font-semibold leading-none">{metrics.inbounds.length}</div>
          </div>
        </div>
        <div class="flex items-center gap-2 rounded-lg border bg-card/60 px-3 py-2">
          <span class="grid h-7 w-7 place-items-center rounded-md bg-emerald-500/15 text-emerald-400">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-3.5"><path d="M3 3v18h18" /><path d="m7 14 4-4 3 3 5-6" /></svg>
          </span>
          <div>
            <div class="text-xs text-muted-foreground">Онлайн</div>
            <div class="text-lg font-semibold leading-none">{metrics.online_count}</div>
          </div>
        </div>
        <div class="flex items-center gap-2 rounded-lg border bg-card/60 px-3 py-2">
          <span class="grid h-7 w-7 place-items-center rounded-md bg-violet-500/15 text-violet-400">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-3.5"><path d="M7 17 17 7M17 7H9m8 0v8" /></svg>
          </span>
          <div>
            <div class="text-xs text-muted-foreground">Трафик</div>
            <div class="text-lg font-semibold leading-none">
              {fmtBytes(metrics.inbounds.reduce((a, m) => a + m.Up + m.Down, 0))}
            </div>
          </div>
        </div>
      </div>
      <div class="mt-4 grid gap-3 sm:grid-cols-2">
        {#each metrics.inbounds as m (m.Tag)}
          <div class="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-3">
            <span
              class="inline-flex items-center gap-1.5 rounded-full border border-emerald-500/40 px-2.5 py-0.5 text-xs font-semibold uppercase tracking-wide text-emerald-300"
            >
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="size-3">
                <path d="M12.6 2.6a2 2 0 0 0-1.2 0L3 6l8.4 3.4a2 2 0 0 0 1.2 0L21 6l-8.4-3.4Z" />
                <path d="m3 12 8.4 3.4a2 2 0 0 0 1.2 0L21 12M3 18l8.4 3.4a2 2 0 0 0 1.2 0L21 18" />
              </svg>
              {m.Tag}
            </span>
            <div class="mt-1.5 grid grid-cols-2 gap-4 pl-1">
              <div>
                <div class="text-xs font-medium text-emerald-400">↑ UP</div>
                <div class="font-mono text-sm">{fmtBytes(m.Up)}</div>
              </div>
              <div>
                <div class="text-xs font-medium text-sky-400">↓ DOWN</div>
                <div class="font-mono text-sm">{fmtBytes(m.Down)}</div>
              </div>
            </div>
          </div>
        {:else}
          <p class="text-sm text-muted-foreground">Счётчиков пока нет</p>
        {/each}
      </div>
    {:else}
      <p class="mt-2 text-sm text-muted-foreground">Загрузка…</p>
    {/if}
    {#if metricsErr}
      <p class="mt-2 text-sm text-red-400">{metricsErr}</p>
    {/if}
  </div>
{/if}

{#if !showMetrics}
<p class="mt-2 text-sm text-yellow-400">Внимание: применение изменений = рестарт xray ~1–2 с</p>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{/if}

<div class="mt-4 overflow-x-auto rounded-xl border">
  <table class="w-full text-sm">
    <thead class="bg-muted/50 text-left text-muted-foreground">
      <tr>
        <th class="px-3 py-2 font-medium">Tag</th>
        <th class="px-3 py-2 font-medium">Порт</th>
        <th class="px-3 py-2 font-medium">Path</th>
        <th class="px-3 py-2 font-medium">Включён</th>
        <th class="px-3 py-2 font-medium">Действия</th>
      </tr>
    </thead>
    <tbody>
      {#each inbounds as inb (inb.id)}
        <tr class="border-t">
          <td class="px-3 py-2 font-medium">{inb.tag}</td>
          <td class="px-3 py-2">{cfgPort(inb)}</td>
          <td class="px-3 py-2">{cfgPath(inb)}</td>
          <td class="px-3 py-2">
            <input
              type="checkbox"
              checked={inb.enabled}
              onchange={(e) => toggle(inb, e.currentTarget.checked)}
              class="size-4 accent-primary"
            />
          </td>
          <td class="px-3 py-2">
            <div class="flex flex-wrap gap-1">
              <Button size="xs" variant="outline" onclick={() => (editing = inb)}>Изменить</Button>
              <Button size="xs" variant="outline" onclick={() => regenKeys(inb)}>Ключи</Button>
              <Button size="xs" variant="destructive" onclick={() => remove(inb)}>Удалить</Button>
            </div>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>
{/if}

{#if editing}
  <InboundForm
    inbound={editing === 'create' ? null : editing}
    onsaved={closeAndReload}
    onclose={() => (editing = null)}
  />
{/if}
