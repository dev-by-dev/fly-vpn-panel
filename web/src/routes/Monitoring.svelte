<script lang="ts">
	import { onMount } from 'svelte';
	import { apiFetch } from '$lib/api';
	import { Button } from '$lib/components/ui/button';
	import { dateTag, t } from '$lib/i18n.svelte';
	import { errText } from '$lib/errors';

	type Entry = { time: string; source: string; level: string; message: string };
	type Source = 'server' | 'xray' | 'caddy';

	const sources: Source[] = ['server', 'xray', 'caddy'];

	let view: 'logs' | 'caddy' = $state('logs');
	let source: Source = $state('server');
	let auto = $state(false);
	let entries: Entry[] = $state([]);
	let error = $state('');
	let loading = $state(false);

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

	// Автообновление: обновляем только видимый список логов, графики ходят по кнопке.
	$effect(() => {
		if (!auto || view !== 'logs') return;
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
		<p class="text-sm text-muted-foreground">{t('monitor.caddyEmpty')}</p>
	</section>
{/if}