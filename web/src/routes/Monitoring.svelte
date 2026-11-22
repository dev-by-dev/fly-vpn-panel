<script lang="ts">
	import { onMount } from 'svelte';
	import { apiFetch } from '$lib/api';
	import { Button } from '$lib/components/ui/button';
	import { dateTag, i18n, t } from '$lib/i18n.svelte';
	import { errText } from '$lib/errors';
	import { fmtBytes } from '$lib/traffic';

	type Entry = { time: string; source: string; level: string; message: string };
	type Source = 'server' | 'xray' | 'caddy';
	type Range = '1h' | '6h' | '24h' | '7d';
	type Point = {
		ts: string;
		rps: number;
		errors_per_s: number;
		avg_ms: number;
		inflight: number;
		rss: number;
		upstream_healthy: boolean | null;
	};
	type Summary = {
		requests?: number;
		errors?: number;
		avg_ms?: number;
		inflight?: number;
		rss?: number;
		upstream_healthy?: boolean | null;
	};

	const sources: Source[] = ['server', 'xray', 'caddy'];

	let view: 'logs' | 'caddy' = $state('logs');
	let source: Source = $state('server');
	let auto = $state(false);
	let paused = $state(false);
	let entries: Entry[] = $state([]);
	let error = $state('');
	let loading = $state(false);

	const ranges: Range[] = ['1h', '6h', '24h', '7d'];
	let range: Range = $state('1h');
	let points: Point[] = $state([]);
	let summary: Summary = $state({});
	let caddyErr = $state('');

	// uPlot живёт вне реактивности: данные заливаем императивно.
	type Chart = { destroy(): void; setSize(size: { width: number; height: number }): void };
	let UPlotMod: (new (conf: unknown, data: number[][], node: HTMLElement) => Chart) & {
		paths: { spline(): unknown };
	} | null = null;
	let elA = $state<HTMLDivElement | null>(null);
	let elB = $state<HTMLDivElement | null>(null);
	let chartA: Chart | null = null;
	let resize: ResizeObserver | null = null;
	let chartB: Chart | null = null;

	async function loadLogs() {
		loading = true;
		error = '';
		try {
			const res = await apiFetch(`/api/monitoring/logs?source=${source}&lines=200`);
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				error = errText(data.error ?? '', t('monitor.loadFail', { status: res.status }));
				entries = [];
				return;
			}
			entries = data.entries ?? [];
		} catch {
			error = t('common.network');
		} finally {
			loading = false;
		}
	}

	function pickSource(s: Source) {
		source = s;
		void loadLogs();
	}

	// Журнал systemd панели недоступен на запись: чистим экран, файл xray режем сами.
	async function clearLogs() {
		entries = [];
		if (source !== 'xray') return;
		try {
			const res = await apiFetch('/api/monitoring/logs?source=xray', { method: 'DELETE' });
			if (!res.ok) {
				const data = await res.json().catch(() => ({}));
				error = errText(data.error ?? '', t('monitor.loadFail', { status: res.status }));
			}
		} catch {
			error = t('common.network');
		}
	}

	async function loadCaddy() {
		caddyErr = '';
		try {
			const res = await apiFetch(`/api/monitoring/caddy?range=${range}`);
			const data = await res.json().catch(() => ({}));
			if (!res.ok) {
				caddyErr = errText(data.error ?? '', t('monitor.loadFail', { status: res.status }));
				points = [];
				return;
			}
			points = data.points ?? [];
			summary = data.summary ?? {};
		} catch {
			caddyErr = t('common.network');
		}
	}

	function chartOpts(labels: [string, string], fmt: (v: number) => string, width: number) {
		return {
			width,
			height: 200,
			class: 'uplot-dark',
			series: [
				{},
				{
					label: labels[0],
					stroke: '#34d399',
					fill: 'rgba(52,211,153,0.15)',
					value: (_u: unknown, v: number) => (v == null ? '—' : fmt(v)),
				},
				{
					label: labels[1],
					stroke: '#60a5fa',
					fill: 'rgba(96,165,250,0.15)',
					value: (_u: unknown, v: number) => (v == null ? '—' : fmt(v)),
				},
			],
			scales: { x: { time: true } },
			axes: [
				{
					stroke: '#64748b',
					grid: { stroke: 'rgba(100,116,139,0.15)' },
					ticks: { stroke: '#64748b' },
					values: (_u: unknown, splits: number[]) =>
						splits.map((s) =>
							new Date(s * 1000).toLocaleTimeString(dateTag(), {
								hour: '2-digit',
								minute: '2-digit',
								hour12: false,
							}),
						),
				},
				{
					stroke: '#64748b',
					grid: { stroke: 'rgba(100,116,139,0.15)' },
					ticks: { stroke: '#64748b' },
					size: 80,
					values: (_u: unknown, splits: number[]) => splits.map((v) => fmt(v)),
				},
			],
			legend: { live: false },
		};
	}

	async function buildCharts() {
		const a = elA;
		const b = elB;
		if (!a || !b || points.length === 0) return;
		UPlotMod ??= (await import('uplot')).default as unknown as typeof UPlotMod;
		if (!UPlotMod) return;
		const x = points.map((p) => Math.floor(new Date(p.ts).getTime() / 1000));
		const requests = [x, points.map((p) => p.rps), points.map((p) => p.errors_per_s)];
		const timing = [
			x,
			points.map((p) => p.avg_ms),
			points.map((p) => p.rss / 1024 / 1024),
		];
		chartA?.destroy();
		chartB?.destroy();
		// uPlot берёт width буквально и сам не мерит контейнер.
		const width = a.clientWidth || 600;
		chartA = new UPlotMod(chartOpts([t('monitor.rps'), t('monitor.errors')], (v) => v.toFixed(2), width), requests, a);
		chartB = new UPlotMod(chartOpts([t('monitor.avgMs'), t('monitor.rssMb')], (v) => v.toFixed(1), width), timing, b);
		resize?.disconnect();
		resize = new ResizeObserver(() => {
			const w = a.clientWidth || 600;
			chartA?.setSize({ width: w, height: 200 });
			chartB?.setSize({ width: w, height: 200 });
		});
		resize.observe(a);
	}

	// Графики пересобираем при смене вида, диапазона и языка (легенда uPlot статична).
	$effect(() => {
		void view;
		void range;
		void i18n.lang;
		const count = points.length;
		if (view !== 'caddy' || count === 0) return;
		void buildCharts();
		return () => {
			resize?.disconnect();
			resize = null;
			chartA?.destroy();
			chartB?.destroy();
			chartA = null;
			chartB = null;
		};
	});

	$effect(() => {
		if (view === 'caddy') void loadCaddy();
	});

	// Автообновление: обновляем только видимый список логов, графики ходят по кнопке.
	$effect(() => {
		if (!auto || paused || view !== 'logs') return;
		const id = setInterval(() => void loadLogs(), 5000);
		return () => clearInterval(id);
	});

	onMount(() => void loadLogs());

	function fmtTime(iso: string): string {
		if (!iso) return '';
		const d = new Date(iso);
		if (Number.isNaN(d.getTime()) || d.getUTCFullYear() < 2000) return '';
		return d.toLocaleTimeString(dateTag(), { hour: '2-digit', minute: '2-digit', second: '2-digit' });
	}

	function levelClass(level: string): string {
		if (level === 'err' || level === 'crit' || level === 'alert' || level === 'emerg') return 'text-red-400';
		if (level === 'warning') return 'text-amber-400';
		return 'text-muted-foreground';
	}
</script>

<div class="flex items-center justify-between">
	<h1 class="text-2xl font-semibold">{t('monitor.title')}</h1>
</div>

<div class="mt-4 flex gap-1">
	<button
		type="button"
		class="rounded-lg px-3 py-1.5 text-sm {view === 'logs'
			? 'bg-muted font-medium text-foreground'
			: 'text-muted-foreground hover:text-foreground'}"
		onclick={() => (view = 'logs')}>{t('monitor.logs')}</button
	>
	<button
		type="button"
		class="rounded-lg px-3 py-1.5 text-sm {view === 'caddy'
			? 'bg-muted font-medium text-foreground'
			: 'text-muted-foreground hover:text-foreground'}"
		onclick={() => (view = 'caddy')}>{t('monitor.caddy')}</button
	>
</div>

{#if view === 'logs'}
	<section class="mt-4 rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
		<div class="flex flex-wrap items-center gap-3">
			<div class="flex gap-1">
				{#each sources as s (s)}
					<button
						type="button"
						class="rounded px-2 py-1 font-mono text-xs {source === s
							? 'bg-muted font-semibold text-foreground'
							: 'text-muted-foreground hover:text-foreground'}"
						onclick={() => pickSource(s)}>{s}</button
					>
				{/each}
			</div>
			<label class="ml-auto flex items-center gap-2 text-sm text-muted-foreground">
				<input type="checkbox" bind:checked={auto} />
				{t('monitor.auto')}
			</label>
			<Button variant="outline" onclick={() => void loadLogs()}>{t('common.refresh')}</Button>
			<Button variant="outline" onclick={() => (paused = !paused)}
				>{paused ? t('monitor.resume') : t('monitor.pause')}</Button
			>
			<Button variant="outline" onclick={() => void clearLogs()}>{t('monitor.clear')}</Button>
		</div>

		{#if error}
			<p class="mt-3 text-sm text-red-400">{error}</p>
		{:else if entries.length === 0}
			<p class="mt-3 text-sm text-muted-foreground">{t('monitor.empty')}</p>
		{:else}
			<div class="mt-3 max-h-[60vh] overflow-auto rounded-lg bg-muted/30 p-3 font-mono text-xs">
				{#each entries as e, i (i)}
					<div class="flex gap-3 border-b border-border/40 py-0.5 last:border-0">
						<span class="shrink-0 text-muted-foreground">{fmtTime(e.time)}</span>
						<span class="w-14 shrink-0 {levelClass(e.level)}">{e.level}</span>
						<span class="whitespace-pre-wrap break-all">{e.message}</span>
					</div>
				{/each}
			</div>
		{/if}
	</section>
{:else}
	<section class="mt-4 rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
		<div class="flex flex-wrap items-center gap-1">
			{#each ranges as r (r)}
				<button
					type="button"
					class="rounded px-2 py-1 text-xs {range === r
						? 'bg-muted font-semibold text-foreground'
						: 'text-muted-foreground hover:text-foreground'}"
					onclick={() => (range = r)}>{r}</button
				>
			{/each}
			<Button class="ml-auto" variant="outline" onclick={() => void loadCaddy()}
				>{t('common.refresh')}</Button
			>
		</div>

		{#if caddyErr}
			<p class="mt-3 text-sm text-red-400">{caddyErr}</p>
		{:else if points.length === 0}
			<p class="mt-3 text-sm text-muted-foreground">{t('monitor.caddyEmpty')}</p>
		{:else}
			<div class="mt-4 grid grid-cols-2 gap-3 md:grid-cols-3 xl:grid-cols-6">
				{#each [
					[t('monitor.requests'), String(summary.requests ?? 0)],
					[t('monitor.errorsTotal'), String(summary.errors ?? 0)],
					[t('monitor.avgMs'), `${summary.avg_ms ?? 0}`],
					[t('monitor.inflight'), String(summary.inflight ?? 0)],
					[t('monitor.rss'), fmtBytes(summary.rss ?? 0)],
					[
						t('monitor.upstream'),
						summary.upstream_healthy === true ? 'ok' : summary.upstream_healthy === false ? 'down' : '—',
					],
				] as [string, string][] as cards (cards[0])}
					<div class="rounded-lg border border-sky-500/10 bg-muted/20 p-3">
						<div class="text-xs text-muted-foreground">{cards[0]}</div>
						<div class="mt-1 text-lg font-semibold">{cards[1]}</div>
					</div>
				{/each}
			</div>

			<h2 class="mt-6 text-sm font-medium">{t('monitor.chartRequests')}</h2>
			<div bind:this={elA} class="mt-2"></div>
			<h2 class="mt-6 text-sm font-medium">{t('monitor.chartTiming')}</h2>
			<div bind:this={elB} class="mt-2"></div>
		{/if}
	</section>
{/if}