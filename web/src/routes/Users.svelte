<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import UserDialog from '$lib/components/UserDialog.svelte';
  import { usersIntent } from '$lib/usersIntent.svelte';

  type UserRow = {
    id: number;
    username: string;
    expiry: string;
    quota_bytes: number;
    quota_period: string;
    device_limit: number;
    disabled: boolean;
    disabled_reason: string;
    used_bytes: number;
    online_count: number;
    device_count: number;
    last_seen: string;
  };

  let users = $state<UserRow[]>([]);
  let error = $state('');
  let dialog = $state<'create' | UserRow | null>(null);
  let sub = $state<{ username: string; url: string; formats: string[] } | null>(null);
  let copied = $state(false);
  // HWID/устройства: отдельный вид из меню вкладки «Юзеры».
  type DeviceRow = { Username: string; IP: string; FirstSeen: string; LastSeen: string; online: boolean };
  let showDevices = $state(false);
  let devices = $state<DeviceRow[]>([]);
  let devicesErr = $state('');

  async function loadDevices() {
    try {
      const res = await apiFetch('/api/devices');
      if (!res.ok) {
        devicesErr = `Не удалось загрузить устройства (${res.status})`;
        return;
      }
      devices = (await res.json()) as DeviceRow[];
      devicesErr = '';
    } catch {
      devicesErr = 'Сеть недоступна';
    }
  }

  // Намерение из шапки: одноразовое действие при переходе на страницу.
  $effect(() => {
    if (usersIntent.view !== 'devices') return;
    usersIntent.view = null;
    showDevices = true;
    void loadDevices();
  });

  const statusLabels: Record<string, string> = {
    quota: 'Квота',
    device_limit: 'Лимит устройств',
    expired: 'Истёк',
    manual: 'Отключён',
  };

  // Отключённый юзер без причины всё равно «Отключён», не «Активен».
  function statusLabel(u: UserRow): string {
    if (!u.disabled) return 'Активен';
    return statusLabels[u.disabled_reason] ?? 'Отключён';
  }

  // Счётчики для карточек: причины отключения не пересекаются.
  const stats = $derived.by(() => {
    const s = { total: users.length, active: 0, expired: 0, limited: 0, disabled: 0 };
    for (const u of users) {
      if (!u.disabled) s.active++;
      else if (u.disabled_reason === 'expired') s.expired++;
      else if (u.disabled_reason === 'quota') s.limited++;
      else s.disabled++;
    }
    return s;
  });

  function fmtBytes(n: number): string {
    if (n <= 0) return '0 Б';
    const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
    const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    const v = (n / 1024 ** i).toFixed(i === 0 ? 0 : 1).replace(/\.0$/, '');
    return `${v} ${units[i]}`;
  }

  function pct(u: UserRow): number {
    if (u.quota_bytes <= 0) return 0;
    return Math.min(100, Math.round((u.used_bytes / u.quota_bytes) * 100));
  }

  // ID юзера по имени — в списке устройств xray отдаёт только username.
  function userId(name: string): number | string {
    return users.find((u) => u.username === name)?.id ?? '—';
  }

  // Относительное «сколько назад» для подписи под ником.
  function fmtAgo(iso: string): string {
    const t = Date.parse(iso);
    if (!t) return 'никогда';
    const s = Math.max(0, Math.round((Date.now() - t) / 1000));
    if (s < 60) return 'несколько секунд назад';
    const m = Math.round(s / 60);
    if (m < 60) return `${m} мин назад`;
    const h = Math.round(m / 60);
    if (h < 24) return `${h} ч назад`;
    return `${Math.round(h / 24)} дн назад`;
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'short' });
  }

  async function load() {
    try {
      const res = await apiFetch('/api/users');
      if (!res.ok) {
        error = `Не удалось загрузить (${res.status})`;
        return;
      }
      users = (await res.json()) as UserRow[];
      error = '';
    } catch {
      error = 'Сеть недоступна';
    }
  }

  async function closeAndReload() {
    dialog = null;
    await load();
  }

  onMount(load);

  async function resetQuota(u: UserRow) {
    await apiFetch(`/api/users/${u.id}/reset-quota`, { method: 'POST' });
    await load();
  }

  async function remove(u: UserRow) {
    if (!confirm(`Удалить ${u.username}?`)) return;
    await apiFetch(`/api/users/${u.id}`, { method: 'DELETE' });
    sub = null;
    await load();
  }

  async function showSub(u: UserRow) {
    const res = await apiFetch(`/api/users/${u.id}/sub`);
    if (!res.ok) return;
    const data = (await res.json()) as { url: string; formats: string[] };
    sub = { username: u.username, url: data.url, formats: data.formats };
    copied = false;
  }

  async function copyUrl() {
    if (!sub) return;
    await navigator.clipboard?.writeText(sub.url).catch(() => {});
    copied = true;
  }
</script>

<div class="flex items-center justify-between">
  <h1 class="text-2xl font-semibold">Пользователи</h1>
  <div class="flex gap-2">
    <Button variant="outline" onclick={load}>Обновить</Button>
    <Button onclick={() => (dialog = 'create')}>Добавить</Button>
  </div>
</div>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{/if}

<!-- Счётчики по статусам -->
{#if !showDevices}
<div class="mt-4 grid gap-3 sm:grid-cols-3 xl:grid-cols-5">
  <div class="flex items-center gap-3 rounded-xl border bg-card px-4 py-3">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-sky-500/10 text-sky-400">
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path
          d="M22 21v-2a4 4 0 0 0-3-3.87M16 3.13a4 4 0 0 1 0 7.75"
        />
      </svg>
    </span>
    <div>
      <div class="text-sm text-muted-foreground">Всего</div>
      <div class="text-2xl font-semibold leading-none">{stats.total}</div>
    </div>
  </div>
  <div class="flex items-center gap-3 rounded-xl border bg-card px-4 py-3">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-emerald-500/10 text-emerald-400">
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M3 12h4l3 8 4-16 3 8h4" />
      </svg>
    </span>
    <div>
      <div class="text-sm text-muted-foreground">Активные</div>
      <div class="text-2xl font-semibold leading-none">{stats.active}</div>
    </div>
  </div>
  <div class="flex items-center gap-3 rounded-xl border bg-card px-4 py-3">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-red-500/10 text-red-400">
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
      </svg>
    </span>
    <div>
      <div class="text-sm text-muted-foreground">Истекли</div>
      <div class="text-2xl font-semibold leading-none">{stats.expired}</div>
    </div>
  </div>
  <div class="flex items-center gap-3 rounded-xl border bg-card px-4 py-3">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-amber-500/10 text-amber-400">
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M6 2h9l5 5v15H6z" /><path d="M15 2v5h5" /><path d="M9 13h6" />
      </svg>
    </span>
    <div>
      <div class="text-sm text-muted-foreground">Квота</div>
      <div class="text-2xl font-semibold leading-none">{stats.limited}</div>
    </div>
  </div>
  <div class="flex items-center gap-3 rounded-xl border bg-card px-4 py-3">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-slate-500/10 text-slate-400">
      <svg class="h-5 w-5" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
        <circle cx="12" cy="12" r="9" /><path d="m5.6 5.6 12.8 12.8" />
      </svg>
    </span>
    <div>
      <div class="text-sm text-muted-foreground">Отключены</div>
      <div class="text-2xl font-semibold leading-none">{stats.disabled}</div>
    </div>
  </div>
</div>

<div class="mt-4 overflow-x-auto rounded-xl border">
  <table class="w-full text-sm">
    <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
      <tr>
        <th class="px-3 py-2 font-medium">Юзернейм</th>
        <th class="px-3 py-2 font-medium">ID</th>
          <th class="px-3 py-2 font-medium">Σ</th>
        <th class="px-3 py-2 font-medium">Статус</th>
        <th class="px-3 py-2 font-medium">Онлайн</th>
        <th class="px-3 py-2 font-medium">Истекает</th>
        <th class="px-3 py-2 font-medium">Израсходовано</th>
        <th class="px-3 py-2 font-medium">Действия</th>
      </tr>
    </thead>
    <tbody>
      {#each users as u (u.id)}
        <tr class="border-t hover:bg-muted/30">
          <td class="px-3 py-2">
            <div class="flex items-center gap-2">
              <span
                class="h-2.5 w-2.5 shrink-0 rounded-full {u.online_count > 0
                  ? 'bg-emerald-400'
                  : 'bg-red-400'}"
              ></span>
              <div class="min-w-0">
                <div class="font-medium">{u.username}</div>
                <div class="text-xs uppercase tracking-wide text-muted-foreground">
                  {fmtAgo(u.last_seen)}
                </div>
              </div>
            </div>
          </td>
          <td class="px-3 py-2 text-muted-foreground">{u.id}</td>
          <td class="px-3 py-2">{u.device_count}</td>
          <td class="px-3 py-2">
            <span
              class="inline-flex rounded-md px-2 py-0.5 text-xs font-medium {u.disabled
                ? 'bg-red-500/15 text-red-400'
                : 'bg-emerald-500/15 text-emerald-400'}"
            >
              {statusLabel(u).toUpperCase()}
            </span>
          </td>
          <td class="px-3 py-2">{u.online_count}</td>
          <td class="px-3 py-2">{fmtDate(u.expiry)}</td>
          <td class="px-3 py-2">
            <div class="flex items-center gap-2">
              <div class="h-1.5 w-28 overflow-hidden rounded-full bg-muted">
                <div
                  class="h-full {pct(u) >= 90 ? 'bg-red-400' : 'bg-emerald-400'}"
                  style="width:{pct(u)}%"
                ></div>
              </div>
              <span class="whitespace-nowrap text-xs text-muted-foreground">
                {fmtBytes(u.used_bytes)} / {u.quota_bytes > 0 ? fmtBytes(u.quota_bytes) : '∞'}
              </span>
            </div>
          </td>
          <td class="px-3 py-2">
            <div class="flex flex-wrap gap-1">
              <Button size="xs" variant="outline" onclick={() => showSub(u)}>Подписка</Button>
              <Button size="xs" variant="outline" onclick={() => resetQuota(u)}>Сброс квоты</Button>
              <Button size="xs" variant="outline" onclick={() => (dialog = u)}>Изменить</Button>
              <Button size="xs" variant="destructive" onclick={() => remove(u)}>Удалить</Button>
            </div>
          </td>
        </tr>
      {:else}
        <tr><td class="px-3 py-4 text-muted-foreground" colspan="7">Пользователей нет</td></tr>
      {/each}
    </tbody>
  </table>
</div>

{:else}
  <div class="mt-4 rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
    <div class="flex items-center justify-between">
      <div>
        <h2 class="font-medium">HWID / устройства</h2>
        <p class="text-xs text-muted-foreground">
          xray отдаёт только имя и IP; платформа, версия и UA недоступны
        </p>
      </div>
      <div class="flex gap-2">
        <Button size="xs" variant="outline" onclick={loadDevices}>Обновить</Button>
        <Button size="xs" variant="outline" onclick={() => (showDevices = false)}>К списку</Button>
      </div>
    </div>
    {#if devicesErr}
      <p class="mt-2 text-sm text-red-400">{devicesErr}</p>
    {/if}
    <div class="mt-3 overflow-x-auto rounded-lg border">
      <table class="w-full text-sm">
        <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th class="px-3 py-2 font-medium">Юзер</th>
            <th class="px-3 py-2 font-medium">ID</th>
            <th class="px-3 py-2 font-medium">HWID / IP</th>
            <th class="px-3 py-2 font-medium">Платформа</th>
            <th class="px-3 py-2 font-medium">Версия</th>
            <th class="px-3 py-2 font-medium">Модель</th>
            <th class="px-3 py-2 font-medium">User-Agent</th>
            <th class="px-3 py-2 font-medium">Первый вход</th>
            <th class="px-3 py-2 font-medium">Активность</th>
          </tr>
        </thead>
        <tbody>
          {#each devices as d (d.Username + d.IP)}
            <tr class="border-t hover:bg-muted/30">
              <td class="px-3 py-2">
                <div class="flex items-center gap-2">
                  <span
                    class="h-2.5 w-2.5 shrink-0 rounded-full {d.online ? 'bg-emerald-400' : 'bg-red-400'}"
                  ></span>
                  <span class="font-medium">{d.Username}</span>
                </div>
              </td>
              <td class="px-3 py-2 text-muted-foreground">{userId(d.Username)}</td>
              <td class="px-3 py-2 font-mono text-xs">{d.IP}</td>
              <td class="px-3 py-2 text-muted-foreground">—</td>
              <td class="px-3 py-2 text-muted-foreground">—</td>
              <td class="px-3 py-2 text-muted-foreground">—</td>
              <td class="px-3 py-2 text-muted-foreground">—</td>
              <td class="px-3 py-2 text-muted-foreground">{fmtDate(d.FirstSeen)}</td>
              <td class="px-3 py-2">
                <div class="text-xs uppercase tracking-wide text-muted-foreground">
                  {fmtAgo(d.LastSeen)}
                </div>
              </td>
            </tr>
          {:else}
            <tr><td class="px-3 py-2 text-muted-foreground" colspan="4">Устройств пока нет</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/if}

{#if sub}
  <div class="mt-4 rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
    <h2 class="font-medium">Подписка: {sub.username}</h2>
    <div class="mt-2 flex items-center gap-2">
      <code class="flex-1 break-all rounded bg-muted px-2 py-1 text-xs">{sub.url}</code>
      <Button size="sm" variant="outline" onclick={copyUrl}>{copied ? 'Скопировано' : 'Копировать'}</Button>
    </div>
    <div class="mt-2 flex flex-wrap gap-3 text-sm">
      {#each sub.formats as f (f)}
        <a class="text-primary underline-offset-4 hover:underline" href="{sub.url}/{f}">{f}</a>
      {/each}
    </div>
  </div>
{/if}

{#if dialog}
  <UserDialog user={dialog === 'create' ? null : dialog} onsaved={closeAndReload} onclose={() => (dialog = null)} />
{/if}