<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import UserDialog from '$lib/components/UserDialog.svelte';

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
  };

  let users = $state<UserRow[]>([]);
  let error = $state('');
  let dialog = $state<'create' | UserRow | null>(null);
  let sub = $state<{ username: string; url: string; formats: string[] } | null>(null);
  let copied = $state(false);

  const statusLabels: Record<string, string> = {
    '': 'Активен',
    quota: 'Квота',
    device_limit: 'Лимит устройств',
    expired: 'Истёк',
    manual: 'Отключён',
  };

  function fmtBytes(n: number): string {
    if (n <= 0) return '0 Б';
    const units = ['Б', 'КБ', 'МБ', 'ГБ', 'ТБ'];
    const i = Math.min(units.length - 1, Math.floor(Math.log(n) / Math.log(1024)));
    const v = (n / 1024 ** i).toFixed(i === 0 ? 0 : 1).replace(/\.0$/, '');
    return `${v} ${units[i]}`;
  }

  function traffic(u: UserRow): string {
    return `${fmtBytes(u.used_bytes)} / ${u.quota_bytes > 0 ? fmtBytes(u.quota_bytes) : '∞'}`;
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
  <h1 class="text-2xl font-semibold">Юзеры</h1>
  <Button onclick={() => (dialog = 'create')}>Добавить</Button>
</div>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{/if}

<div class="mt-4 overflow-x-auto rounded-xl border">
  <table class="w-full text-sm">
    <thead class="bg-muted/50 text-left text-muted-foreground">
      <tr>
        <th class="px-3 py-2 font-medium">Имя</th>
        <th class="px-3 py-2 font-medium">Трафик (исп / квота)</th>
        <th class="px-3 py-2 font-medium">Онлайн</th>
        <th class="px-3 py-2 font-medium">Действует до</th>
        <th class="px-3 py-2 font-medium">Статус</th>
        <th class="px-3 py-2 font-medium">Действия</th>
      </tr>
    </thead>
    <tbody>
      {#each users as u (u.id)}
        <tr class="border-t">
          <td class="px-3 py-2 font-medium">{u.username}</td>
          <td class="px-3 py-2">{traffic(u)}</td>
          <td class="px-3 py-2">{u.online_count}</td>
          <td class="px-3 py-2">{fmtDate(u.expiry)}</td>
          <td class="px-3 py-2">
            <span
              class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium {u.disabled
                ? 'bg-red-500/15 text-red-400'
                : 'bg-emerald-500/15 text-emerald-400'}"
            >
              {statusLabels[u.disabled_reason] ?? u.disabled_reason}
            </span>
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
      {/each}
    </tbody>
  </table>
</div>

{#if sub}
  <div class="mt-4 rounded-xl border bg-card p-4">
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
