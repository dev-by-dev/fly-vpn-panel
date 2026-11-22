<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import UserDialog from '$lib/components/UserDialog.svelte';
  import { usersIntent } from '$lib/usersIntent.svelte';
  import { dateTag, t } from '$lib/i18n.svelte';
  import { fmtBytes } from '$lib/traffic';

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
  type DeviceRow = {
    Username: string;
    IP: string;
    FirstSeen: string;
    LastSeen: string;
    online: boolean;
    same_ip: boolean;
    HWID: string;
    Platform: string;
    OSVersion: string;
    Model: string;
    Country: string;
  ASN: string;
  Provider: string;
  UserAgent: string;
  };
  let showDevices = $state(false);
  let devices = $state<DeviceRow[]>([]);
  let devicesErr = $state('');

  async function loadDevices() {
    try {
      const res = await apiFetch('/api/devices');
      if (!res.ok) {
        devicesErr = t('users.devicesLoadFail', { status: res.status });
        return;
      }
      devices = (await res.json()) as DeviceRow[];
      devicesErr = '';
    } catch {
      devicesErr = t('common.network');
    }
  }

  // Намерение из шапки: одноразовое действие при переходе на страницу.
  $effect(() => {
    const view = usersIntent.view;
    if (!view) return;
    usersIntent.view = null;
    // «Юзеры» в меню возвращает список даже если hash уже #/users.
    if (view === 'list') {
      showDevices = false;
      return;
    }
    showDevices = true;
    void loadDevices();
  });

  // Отключённый юзер без причины всё равно «Отключён», не «Активен».
  function statusLabel(u: UserRow): string {
    if (!u.disabled) return t('users.statusActive');
    switch (u.disabled_reason) {
      case 'quota':
        return t('users.statusQuota');
      case 'device_limit':
        return t('users.statusDeviceLimit');
      case 'expired':
        return t('users.statusExpired');
      default:
        return t('users.statusDisabled');
    }
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

  function pct(u: UserRow): number {
    if (u.quota_bytes <= 0) return 0;
    return Math.min(100, Math.round((u.used_bytes / u.quota_bytes) * 100));
  }

  // ID юзера по имени — в списке устройств xray отдаёт только username.
  function userId(name: string): number | string {
    return users.find((u) => u.username === name)?.id ?? '—';
  }

  // Относительное «сколько назад» для подписи под ником.
  // Флаг из ISO-кода страны; для пустого кода — прочерк.
  function flagOf(cc: string): string {
    if (!/^[A-Za-z]{2}$/.test(cc)) return '—';
    return String.fromCodePoint(...[...cc.toUpperCase()].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
  }

  function fmtAgo(iso: string): string {
    const ts = Date.parse(iso);
    if (!ts) return t('users.never');
    const s = Math.max(0, Math.round((Date.now() - ts) / 1000));
    if (s < 60) return t('users.agoNow');
    const m = Math.round(s / 60);
    if (m < 60) return t('users.agoMin', { n: m });
    const h = Math.round(m / 60);
    if (h < 24) return t('users.agoHour', { n: h });
    return t('users.agoDay', { n: Math.round(h / 24) });
  }

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleString(dateTag(), { dateStyle: 'short', timeStyle: 'short' });
  }

  async function load() {
    try {
      const res = await apiFetch('/api/users');
      if (!res.ok) {
        error = t('users.loadFail', { status: res.status });
        return;
      }
      users = (await res.json()) as UserRow[];
      error = '';
    } catch {
      error = t('common.network');
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
    if (!confirm(t('users.confirmDelete', { name: u.username }))) return;
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
  <h1 class="text-2xl font-semibold">{t('users.title')}</h1>
  {#if !showDevices}
    <div class="flex gap-2">
      <Button variant="outline" onclick={load}>{t('common.refresh')}</Button>
      <Button onclick={() => (dialog = 'create')}>{t('users.add')}</Button>
    </div>
  {/if}
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
      <div class="text-sm text-muted-foreground">{t('users.statTotal')}</div>
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
      <div class="text-sm text-muted-foreground">{t('users.statActive')}</div>
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
      <div class="text-sm text-muted-foreground">{t('users.statExpired')}</div>
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
      <div class="text-sm text-muted-foreground">{t('users.statQuota')}</div>
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
      <div class="text-sm text-muted-foreground">{t('users.statDisabled')}</div>
      <div class="text-2xl font-semibold leading-none">{stats.disabled}</div>
    </div>
  </div>
</div>

<div class="mt-4 overflow-x-auto rounded-xl border">
  <table class="w-full text-sm">
    <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
      <tr>
        <th class="px-3 py-2 font-medium">{t('users.colUsername')}</th>
        <th class="px-3 py-2 font-medium">ID</th>
          <th class="px-3 py-2 font-medium">Σ</th>
        <th class="px-3 py-2 font-medium">{t('users.colStatus')}</th>
        <th class="px-3 py-2 font-medium">{t('users.colOnline')}</th>
        <th class="px-3 py-2 font-medium">{t('users.colExpiry')}</th>
        <th class="px-3 py-2 font-medium">{t('users.colUsed')}</th>
        <th class="px-3 py-2 font-medium">{t('users.colActions')}</th>
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
              <Button size="xs" variant="outline" onclick={() => showSub(u)}>{t('users.btnSub')}</Button>
              <Button size="xs" variant="outline" onclick={() => resetQuota(u)}>{t('users.btnResetQuota')}</Button>
              <Button size="xs" variant="outline" onclick={() => (dialog = u)}>{t('users.btnEdit')}</Button>
              <Button size="xs" variant="destructive" onclick={() => remove(u)}>{t('users.btnDelete')}</Button>
            </div>
          </td>
        </tr>
      {:else}
        <tr><td class="px-3 py-4 text-muted-foreground" colspan="7">{t('users.empty')}</td></tr>
      {/each}
    </tbody>
  </table>
</div>

{:else}
  <div class="mt-4 rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
    <div class="flex items-center justify-between">
      <h2 class="font-medium">{t('users.devicesTitle')}</h2>
      <div class="flex gap-2">
        <Button size="xs" variant="outline" onclick={loadDevices}>{t('common.refresh')}</Button>
        <Button size="xs" variant="outline" onclick={() => (showDevices = false)}>{t('users.backToList')}</Button>
      </div>
    </div>
    {#if devicesErr}
      <p class="mt-2 text-sm text-red-400">{devicesErr}</p>
    {/if}
    <div class="mt-3 overflow-x-auto rounded-lg border">
      <table class="w-full text-sm">
        <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th class="px-3 py-2 font-medium">{t('users.colUser')}</th>
            <th class="px-3 py-2 font-medium">ID</th>
            <th class="px-3 py-2 font-medium">{t('users.colIp')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colCountry')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colProvider')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colPlatform')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colVersion')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colModel')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colUa')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colFirstSeen')}</th>
            <th class="px-3 py-2 font-medium">{t('users.colActivity')}</th>
          </tr>
        </thead>
        <tbody>
          {#each devices as d (d.Username + '|' + d.HWID + '|' + d.IP)}
            <tr class="border-t hover:bg-muted/30">
              <td class="px-3 py-2">
                <div class="flex items-center gap-2">
                  <span
                    class="h-2.5 w-2.5 shrink-0 rounded-full {d.online
                      ? 'bg-emerald-400'
                      : d.same_ip
                        ? 'bg-slate-500'
                        : 'bg-red-400'}"
                  ></span>
                  <span class="font-medium">{d.Username}</span>
                </div>
              </td>
              <td class="px-3 py-2 text-muted-foreground">{userId(d.Username)}</td>
              <td class="px-3 py-2 font-mono text-xs">
                {d.IP}
                {#if d.same_ip}
                  <span class="ml-1 text-[10px] uppercase tracking-wide text-muted-foreground">{t('users.sameIp')}</span>
                {/if}
              </td>
              <td class="px-3 py-2 text-xs">{flagOf(d.Country)} {d.Country || '—'}</td>
              <td class="px-3 py-2 text-xs text-muted-foreground">{d.Provider || '—'}</td>
              <td class="px-3 py-2 text-muted-foreground">{d.Platform || '—'}</td>
              <td class="px-3 py-2 text-muted-foreground">{d.OSVersion || '—'}</td>
              <td class="px-3 py-2 text-muted-foreground">{d.Model || '—'}</td>
              <td class="px-3 py-2 text-muted-foreground">{d.UserAgent || '—'}</td>
              <td class="px-3 py-2 text-muted-foreground">{fmtDate(d.FirstSeen)}</td>
              <td class="px-3 py-2">
                <div class="text-xs uppercase tracking-wide text-muted-foreground">
                  {fmtAgo(d.LastSeen)}
                </div>
              </td>
            </tr>
          {:else}
            <tr><td class="px-3 py-2 text-muted-foreground" colspan="11">{t('users.noDevices')}</td></tr>
          {/each}
        </tbody>
      </table>
    </div>
  </div>
{/if}

{#if sub}
  <div class="mt-4 rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
    <h2 class="font-medium">{t('users.subTitle', { name: sub.username })}</h2>
    <div class="mt-2 flex items-center gap-2">
      <code class="flex-1 break-all rounded bg-muted px-2 py-1 text-xs">{sub.url}</code>
      <Button size="sm" variant="outline" onclick={copyUrl}>{copied ? t('users.copied') : t('users.copy')}</Button>
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