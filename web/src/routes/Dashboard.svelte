<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import TrafficChart from '$lib/components/TrafficChart.svelte';
  import { fmtBytes, fmtUptime, type Bucket } from '$lib/traffic';

  type Device = { Username: string; IP: string; FirstSeen: string; LastSeen: string };
  // xray Status has no json tags → Go field names capitalized; Uptime = ns.
  type XrayStatus = { Running: boolean; PID: number; Uptime: number; Restarts: number; Version: string };
  type Summary = { total_up: number; total_down: number; online_count: number };

  let range = $state<'day' | 'week' | 'month'>('day');
  let user = $state('');
  let users = $state<{ username: string }[]>([]);
  let buckets = $state<Bucket[]>([]);
  let devices = $state<Device[]>([]);
  let xray = $state<XrayStatus | null>(null);
  let summary = $state<Summary | null>(null);
  let error = $state('');
  let busy = $state('');

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleString('ru-RU', { dateStyle: 'short', timeStyle: 'medium' });
  }

  async function loadTraffic() {
    try {
      const res = await apiFetch(`/api/stats/traffic?user=${encodeURIComponent(user)}&range=${range}`);
      if (res.ok) buckets = (await res.json()) as Bucket[];
    } catch {
      error = 'Сеть недоступна';
    }
  }

  async function loadRest() {
    try {
      const [dev, st, sum, us] = await Promise.all([
        apiFetch('/api/stats/online'),
        apiFetch('/api/xray/status'),
        apiFetch('/api/stats/summary'),
        apiFetch('/api/users'),
      ]);
      if (dev.ok) devices = (await dev.json()) as Device[];
      if (st.ok) xray = (await st.json()) as XrayStatus;
      if (sum.ok) summary = (await sum.json()) as Summary;
      if (us.ok) users = ((await us.json()) as { username: string }[]).slice(0, 100);
      error = '';
    } catch {
      error = 'Сеть недоступна';
    }
  }

  function reload() {
    loadTraffic();
    loadRest();
  }

  async function xrayAction(action: 'start' | 'stop' | 'restart') {
    const labels = { start: 'запустить', stop: 'остановить', restart: 'перезапустить' };
    if (!window.confirm(`Точно ${labels[action]} xray?`)) return;
    busy = action;
    try {
      const res = await apiFetch(`/api/xray/${action}`, { method: 'POST' });
      if (!res.ok) error = `Ошибка xray ${action} (${res.status})`;
    } catch {
      error = 'Сеть недоступна';
    } finally {
      busy = '';
      const st = await apiFetch('/api/xray/status');
      if (st.ok) xray = (await st.json()) as XrayStatus;
    }
  }

  onMount(reload);
</script>

<div class="flex items-center justify-between">
  <h1 class="text-2xl font-semibold">Дашборд</h1>
  <Button variant="outline" onclick={reload}>Обновить</Button>
</div>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{/if}

<!-- Summary totals -->
<div class="mt-4 grid gap-4 sm:grid-cols-3">
  <div class="rounded-xl border bg-card p-4">
    <div class="text-xs text-muted-foreground">Отдано всего</div>
    <div class="mt-1 text-xl font-semibold">{summary ? fmtBytes(summary.total_up) : '—'}</div>
  </div>
  <div class="rounded-xl border bg-card p-4">
    <div class="text-xs text-muted-foreground">Принято всего</div>
    <div class="mt-1 text-xl font-semibold">{summary ? fmtBytes(summary.total_down) : '—'}</div>
  </div>
  <div class="rounded-xl border bg-card p-4">
    <div class="text-xs text-muted-foreground">Онлайн</div>
    <div class="mt-1 text-xl font-semibold">{summary ? summary.online_count : '—'}</div>
  </div>
</div>

<!-- Traffic chart -->
<div class="mt-4 rounded-xl border bg-card p-4">
  <div class="mb-3 flex flex-wrap items-center gap-3">
    <div class="flex gap-1">
      {#each [['day', 'День'], ['week', 'Неделя'], ['month', 'Месяц']] as [r, label] (r)}
        <Button
          size="xs"
          variant={range === r ? 'default' : 'outline'}
          onclick={() => {
            range = r as typeof range;
            loadTraffic();
          }}>{label}</Button
        >
      {/each}
    </div>
    <select
      class="h-8 rounded-md border bg-background px-2 text-sm"
      value={user}
      onchange={(e) => {
        user = e.currentTarget.value;
        loadTraffic();
      }}
    >
      <option value="">Все юзеры</option>
      {#each users as u (u.username)}
        <option value={u.username}>{u.username}</option>
      {/each}
    </select>
  </div>
  <TrafficChart {buckets} height={260} />
</div>

<!-- Xray status + online devices -->
<div class="mt-4 grid gap-4 lg:grid-cols-2">
  <div class="rounded-xl border bg-card p-4">
    <h2 class="font-medium">xray</h2>
    {#if xray}
      <div class="mt-2 space-y-1 text-sm">
        <div class="flex items-center gap-2">
          <span
            class="inline-flex rounded-full px-2 py-0.5 text-xs font-medium {xray.Running
              ? 'bg-emerald-500/15 text-emerald-400'
              : 'bg-red-500/15 text-red-400'}"
          >
            {xray.Running ? 'Работает' : 'Остановлен'}
          </span>
          <span class="text-muted-foreground">PID {xray.PID}</span>
        </div>
        <div>Версия: {xray.Version || '—'}</div>
        <div>Аптайм: {fmtUptime(xray.Uptime)}</div>
        <div>Рестартов: {xray.Restarts}</div>
      </div>
      <div class="mt-3 flex gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={busy !== '' || xray.Running}
          onclick={() => xrayAction('start')}>Запустить</Button
        >
        <Button
          size="sm"
          variant="outline"
          disabled={busy !== '' || !xray.Running}
          onclick={() => xrayAction('stop')}>Остановить</Button
        >
        <Button
          size="sm"
          variant="destructive"
          disabled={busy !== ''}
          onclick={() => xrayAction('restart')}>Рестарт</Button
        >
      </div>
    {:else}
      <p class="mt-2 text-sm text-muted-foreground">Загрузка…</p>
    {/if}
  </div>

  <div class="rounded-xl border bg-card p-4">
    <h2 class="font-medium">Онлайн-устройства</h2>
    <table class="mt-2 w-full text-sm">
      <thead class="text-left text-muted-foreground">
        <tr>
          <th class="py-1 font-medium">Юзер</th>
          <th class="py-1 font-medium">IP</th>
          <th class="py-1 font-medium">Последняя активность</th>
        </tr>
      </thead>
      <tbody>
        {#each devices as d (d.Username + d.IP)}
          <tr class="border-t">
            <td class="py-1.5 font-medium">{d.Username}</td>
            <td class="py-1.5 font-mono text-xs">{d.IP}</td>
            <td class="py-1.5">{fmtDate(d.LastSeen)}</td>
          </tr>
        {:else}
          <tr><td class="py-2 text-muted-foreground" colspan="3">Никого нет</td></tr>
        {/each}
      </tbody>
    </table>
  </div>
</div>
