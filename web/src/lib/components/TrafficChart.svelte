<script lang="ts">
  import { onMount } from 'svelte';
  import { toSeries, type Bucket } from '$lib/traffic';

  let { buckets = [], height = 260 }: { buckets?: Bucket[]; height?: number } = $props();

  let el = $state<HTMLDivElement | null>(null);
  // chart — non-reactive: mutated imperatively, $effect pushes data into it.
  let chart: import('uplot').default | null = null;

  const opts = (h: number) => ({
    width: 0, // set after mount from el.clientWidth
    height: h,
    class: 'uplot-dark',
    series: [
      {},
      {
        label: 'Отдача',
        stroke: '#34d399',
        fill: 'rgba(52,211,153,0.15)',
        value: (u: unknown, v: number) => (v == null ? '—' : v.toFixed(0)),
      },
      {
        label: 'Загрузка',
        stroke: '#60a5fa',
        fill: 'rgba(96,165,250,0.15)',
        value: (u: unknown, v: number) => (v == null ? '—' : v.toFixed(0)),
      },
    ],
    scales: { x: { time: true } },
    axes: [
      { stroke: '#64748b', grid: { stroke: 'rgba(100,116,139,0.15)' }, ticks: { stroke: '#64748b' } },
      { stroke: '#64748b', grid: { stroke: 'rgba(100,116,139,0.15)' }, ticks: { stroke: '#64748b' } },
    ],
    legend: { live: false },
  });

  onMount(() => {
    let dead = false;
    void (async () => {
      // Dynamic import: uPlot touches canvas/DOM APIs, browser-only.
      const UPlot = (await import('uplot')).default;
      if (dead || !el) return;
      const conf = opts(height);
      conf.width = el.clientWidth || 600;
      chart = new UPlot(conf, toSeries(buckets), el);
    })();
    return () => {
      dead = true;
      chart?.destroy();
      chart = null;
    };
  });

  $effect(() => {
    // buckets read here makes the effect re-run on data reload.
    if (chart) chart.setData(toSeries(buckets));
  });
</script>

<!-- shortcut: no ResizeObserver, chart width fixed at mount -->
<div bind:this={el} class="w-full"></div>
