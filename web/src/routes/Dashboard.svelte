<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import TrafficChart from '$lib/components/TrafficChart.svelte';
  import { fmtBytes, fmtRate, type Bucket } from '$lib/traffic';

  // xray Status has no json tags → Go field names capitalized; Uptime = ns.
  type XrayStatus = { Running: boolean; PID: number; Uptime: number; Restarts: number; Version: string };
  type Summary = { total_up: number; total_down: number; online_count: number };
  type PanelUser = {
    username: string;
    expiry: string;
    quota_bytes: number;
    used_bytes: number;
    disabled: boolean;
  };
  type SystemStats = {
    cpu_used_percent: number;
    cpu_cores: number;
    mem_total: number;
    mem_used: number;
    mem_free: number;
    disk_total: number;
    disk_free: number;
  };

  let range = $state<'day' | 'week' | 'month'>('day');
  let user = $state('');
  let users = $state<PanelUser[]>([]);
  let buckets = $state<Bucket[]>([]);
  // Режим реального времени: сырые сэмплы поллера (байты за интервал), опрос каждые 5 с.
  let live = $state(false);
  let liveBuckets = $state<Bucket[]>([]);
  let liveInterval = $state(30);
  let xray = $state<XrayStatus | null>(null);
  let summary = $state<Summary | null>(null);
  let sys = $state<SystemStats | null>(null);
  let error = $state('');
  let busy = $state('');

  // Счётчики для карточек «Пользователи»: статусы взаимоисключающие, сумма = всего.
  const userStats = $derived.by(() => {
    const now = Date.now();
    let expired = 0, limited = 0, disabled = 0;
    for (const u of users) {
      if (Date.parse(u.expiry) <= now) expired++;
      else if (u.quota_bytes > 0 && u.used_bytes >= u.quota_bytes) limited++;
      else if (u.disabled) disabled++;
    }
    return { total: users.length, expired, limited, disabled, active: users.length - expired - limited - disabled };
  });

  async function loadTraffic() {
    try {
      const res = await apiFetch(`/api/stats/traffic?user=${encodeURIComponent(user)}&range=${range}`);
      if (res.ok) buckets = (await res.json()) as Bucket[];
    } catch {
      error = 'Сеть недоступна';
    }
  }

  // Реальное время: пока тогл включён, тянем последние 15 минут сэмплов каждые 5 с.
  $effect(() => {
    if (!live) return;
    let dead = false;
    const load = async () => {
      try {
        const res = await apiFetch('/api/stats/live?minutes=15');
        if (dead || !res.ok) return;
        const data = (await res.json()) as { interval_sec: number; buckets: Bucket[] };
        liveInterval = data.interval_sec > 0 ? data.interval_sec : 30;
        liveBuckets = data.buckets ?? [];
      } catch {
        if (!dead) error = 'Сеть недоступна';
      }
    };
    void load();
    const timer = setInterval(load, 5000);
    return () => {
      dead = true;
      clearInterval(timer);
    };
  });

  // Байты за интервал → бит/с для отображения скорости.
  const liveBps = $derived(
    liveBuckets.map((b) => ({
      TS: b.TS,
      Up: (b.Up * 8) / liveInterval,
      Down: (b.Down * 8) / liveInterval,
    })),
  );
  const shownBuckets = $derived(live ? liveBps : buckets);
  const liveLast = $derived(liveBps.length ? liveBps[liveBps.length - 1] : null);

  async function loadRest() {
    try {
      const [st, sum, us, sy] = await Promise.all([
        apiFetch('/api/xray/status'),
        apiFetch('/api/stats/summary'),
        apiFetch('/api/users'),
        apiFetch('/api/system'),
      ]);
      if (st.ok) xray = (await st.json()) as XrayStatus;
      if (sum.ok) summary = (await sum.json()) as Summary;
      if (us.ok) users = (await us.json()) as PanelUser[];
      if (sy.ok) sys = (await sy.json()) as SystemStats;
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
  <h1 class="text-2xl font-semibold">Главная</h1>
  <Button variant="outline" onclick={reload}>Обновить</Button>
</div>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{/if}

<!-- Host metrics -->
<h2 class="mt-6 font-medium">Сервер</h2>
<div class="mt-3 flex flex-wrap gap-2">
  <div class="flex min-w-40 items-center gap-3 rounded-xl border bg-card px-3 py-2">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-sky-500/10 text-sky-400">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5"><rect x="4" y="4" width="16" height="16" rx="2" /><rect x="9" y="9" width="6" height="6" /><path d="M9 2v2M15 2v2M9 20v2M15 20v2M2 9h2M2 15h2M20 9h2M20 15h2" /></svg>
    </span>
    <div class="min-w-0">
      <div class="text-xs text-muted-foreground">ЦПУ занято</div>
      <div class="text-lg font-semibold leading-none">{sys ? `${Math.round(sys.cpu_used_percent)}%` : '—'}</div>
      {#if sys}
        <div class="mt-1 text-xs text-muted-foreground">{sys.cpu_cores} ядер · свободно {Math.round(100 - sys.cpu_used_percent)}%</div>
      {/if}
    </div>
  </div>
  <div class="flex min-w-40 items-center gap-3 rounded-xl border bg-card px-3 py-2">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-violet-500/10 text-violet-400">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5"><path d="M6 19v-3M10 19v-3M14 19v-3M18 19v-3" /><rect x="2" y="6" width="20" height="10" rx="1" /></svg>
    </span>
    <div class="min-w-0">
      <div class="text-xs text-muted-foreground">ОЗУ занято</div>
      <div class="text-lg font-semibold leading-none">{sys && sys.mem_total > 0 ? fmtBytes(sys.mem_used) : '—'}</div>
      {#if sys && sys.mem_total > 0}
        <div class="mt-1 text-xs text-muted-foreground">из {fmtBytes(sys.mem_total)} · свободно {fmtBytes(sys.mem_free)}</div>
      {/if}
    </div>
  </div>
  <div class="flex min-w-40 items-center gap-3 rounded-xl border bg-card px-3 py-2">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-amber-500/10 text-amber-400">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5"><line x1="22" x2="2" y1="12" y2="12" /><path d="M5.45 5.11 2 12v6a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2v-6l-3.45-6.89A2 2 0 0 0 16.76 4H7.24a2 2 0 0 0-1.79 1.11z" /><line x1="6" x2="6.01" y1="16" y2="16" /><line x1="10" x2="10.01" y1="16" y2="16" /></svg>
    </span>
    <div class="min-w-0">
      <div class="text-xs text-muted-foreground">Свободно на диске</div>
      <div class="text-lg font-semibold leading-none">{sys ? fmtBytes(sys.disk_free) : '—'}</div>
      {#if sys}
        <div class="mt-1 text-xs text-muted-foreground">из {fmtBytes(sys.disk_total)}</div>
      {/if}
    </div>
  </div>
  <div class="flex min-w-40 items-center gap-3 rounded-xl border bg-card px-3 py-2">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg bg-blue-500/10 text-blue-400">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5"><path d="m17 3 4 4-4 4" /><path d="M21 7H8a5 5 0 0 0-5 5" /><path d="m7 21-4-4 4-4" /><path d="M3 17h13a5 5 0 0 0 5-5" /></svg>
    </span>
    <div class="min-w-0">
      <div class="text-xs text-muted-foreground">Потреблено трафика</div>
      <div class="text-lg font-semibold leading-none">{summary ? fmtBytes(summary.total_up + summary.total_down) : '—'}</div>
      {#if summary}
        <div class="mt-1 text-xs text-muted-foreground">↑ {fmtBytes(summary.total_up)} · ↓ {fmtBytes(summary.total_down)}</div>
      {/if}
    </div>
  </div>
  <div class="flex min-w-40 items-center gap-3 rounded-xl border bg-card px-3 py-2">
    <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg {xray?.Running ? 'bg-emerald-500/10 text-emerald-400' : 'bg-red-500/10 text-red-400'}">
      <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5"><rect x="2" y="2" width="20" height="8" rx="2" /><rect x="2" y="14" width="20" height="8" rx="2" /><line x1="6" x2="6.01" y1="6" y2="6" /><line x1="6" x2="6.01" y1="18" y2="18" /></svg>
    </span>
    <div class="min-w-0">
      <div class="text-xs text-muted-foreground">xray</div>
      <div class="text-lg font-semibold leading-none {xray?.Running ? 'text-emerald-400' : 'text-red-400'}">
        {xray ? (xray.Running ? 'Работает' : 'Остановлен') : '—'}
      </div>
      <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
        <span>{xray?.Version || 'версия —'}</span>
        <div class="flex gap-1">
          <Button size="xs" variant="outline" disabled={busy !== '' || !!xray?.Running} onclick={() => xrayAction('start')}>Старт</Button>
          <Button size="xs" variant="outline" disabled={busy !== '' || !xray?.Running} onclick={() => xrayAction('stop')}>Стоп</Button>
          <Button size="xs" variant="destructive" disabled={busy !== ''} onclick={() => xrayAction('restart')}>Рестарт</Button>
        </div>
      </div>
    </div>
  </div>
</div>

<!-- Summary totals -->
<!-- User stats -->
<h2 class="mt-6 font-medium">Пользователи</h2>
<div class="mt-3 flex flex-wrap gap-2">
  {#each [
    { label: 'Всего', value: userStats.total, box: 'bg-sky-500/10 text-sky-400', icon: 'users' },
    { label: 'Active', value: userStats.active, box: 'bg-emerald-500/10 text-emerald-400', icon: 'pulse' },
    { label: 'Expired', value: userStats.expired, box: 'bg-red-500/10 text-red-400', icon: 'clock' },
    { label: 'Limited', value: userStats.limited, box: 'bg-amber-500/10 text-amber-400', icon: 'hourglass' },
    { label: 'Disabled', value: userStats.disabled, box: 'bg-slate-500/10 text-slate-400', icon: 'ban' },
  ] as c (c.label)}
    <div class="flex min-w-40 items-center gap-3 rounded-xl border bg-card px-3 py-2">
      <span class="grid h-10 w-10 shrink-0 place-items-center rounded-lg {c.box}">
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="h-5 w-5">
          {#if c.icon === 'users'}
            <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" /><path d="M22 21v-2a4 4 0 0 0-3-3.87" /><path d="M16 3.13a4 4 0 0 1 0 7.75" />
          {:else if c.icon === 'pulse'}
            <path d="M22 12h-4l-3 9L9 3l-3 9H2" />
          {:else if c.icon === 'clock'}
            <circle cx="12" cy="12" r="10" /><path d="M12 6v6l4 2" />
          {:else if c.icon === 'hourglass'}
            <path d="M5 22h14" /><path d="M5 2h14" /><path d="M17 22v-4.172a2 2 0 0 0-.586-1.414L12 12l-4.414 4.414A2 2 0 0 0 7 17.828V22" /><path d="M7 2v4.172a2 2 0 0 0 .586 1.414L12 12l4.414-4.414A2 2 0 0 0 17 6.172V2" />
          {:else}
            <circle cx="12" cy="12" r="10" /><path d="m4.9 4.9 14.2 14.2" />
          {/if}
        </svg>
      </span>
      <div class="min-w-0">
        <div class="text-sm text-muted-foreground">{c.label}</div>
        <div class="text-2xl font-semibold leading-none">{c.value}</div>
      </div>
    </div>
  {/each}
</div>

<h2 class="mt-6 font-medium">Трафик панели</h2>
<div class="mt-3 flex flex-wrap gap-2">
  <div class="rounded-lg border bg-card px-3 py-2">
    <div class="flex items-baseline gap-2">
      <span class="text-xs text-muted-foreground">Отдано всего</span>
      <span class="text-lg font-semibold leading-none">{summary ? fmtBytes(summary.total_up) : '—'}</span>
    </div>
  </div>
  <div class="rounded-lg border bg-card px-3 py-2">
    <div class="flex items-baseline gap-2">
      <span class="text-xs text-muted-foreground">Принято всего</span>
      <span class="text-lg font-semibold leading-none">{summary ? fmtBytes(summary.total_down) : '—'}</span>
    </div>
  </div>
  <div class="rounded-lg border bg-card px-3 py-2">
    <div class="flex items-baseline gap-2">
      <span class="text-xs text-muted-foreground">Онлайн</span>
      <span class="text-lg font-semibold leading-none">{summary ? summary.online_count : '—'}</span>
    </div>
  </div>
</div>

<!-- Traffic chart -->
<div class="mt-4 rounded-xl border bg-card p-4">
  <div class="mb-3 flex flex-wrap items-center gap-3">
    <div class="flex gap-1">
      {#each [['day', 'День'], ['week', 'Неделя'], ['month', 'Месяц']] as [r, label] (r)}
        <Button
          size="xs"
          variant={!live && range === r ? 'default' : 'outline'}
          onclick={() => {
            live = false;
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
        live = false;
        user = e.currentTarget.value;
        loadTraffic();
      }}
    >
      <option value="">Все юзеры</option>
      {#each users as u (u.username)}
        <option value={u.username}>{u.username}</option>
      {/each}
    </select>

    <label class="ml-auto flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
      <input type="checkbox" bind:checked={live} class="peer sr-only" />
      <span
        class="relative h-5 w-9 rounded-full bg-muted transition-colors after:absolute after:left-0.5 after:top-0.5 after:h-4 after:w-4 after:rounded-full after:bg-background after:transition-transform peer-checked:bg-primary peer-checked:after:translate-x-4"
      ></span>
      В реальном времени
    </label>
    {#if live && liveLast}
      <span class="text-sm text-muted-foreground">
        ↓ Входящий {fmtRate(liveLast.Down)} · ↑ Исходящий {fmtRate(liveLast.Up)}
      </span>
    {/if}
  </div>
  <TrafficChart buckets={shownBuckets} rate={live} height={260} />
</div>
