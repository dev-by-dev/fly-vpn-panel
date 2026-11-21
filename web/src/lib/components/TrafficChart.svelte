<script lang="ts">
  import { onMount } from 'svelte';
  // Без CSS uPlot не позиционирует ось/легенду: они наезжают на график и соседние блоки.
  import 'uplot/dist/uPlot.min.css';
  import { toSeries, fmtBytes, type Bucket } from '$lib/traffic';

  let { buckets = [], height = 260 }: { buckets?: Bucket[]; height?: number } = $props();

  let el = $state<HTMLDivElement | null>(null);
  // chart — non-reactive: mutated imperatively, $effect pushes data into it.
  // Structural type: пакет uplot отдаёт класс через `export =`, InstanceType на нём не выводится.
  type Chart = { setData(d: number[][]): void; setSize(s: { width: number; height: number }): void; destroy(): void };
  let chart: Chart | null = null;

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
        value: (u: unknown, v: number) => (v == null ? '—' : fmtBytes(v)),
      },
      {
        label: 'Загрузка',
        stroke: '#60a5fa',
        fill: 'rgba(96,165,250,0.15)',
        value: (u: unknown, v: number) => (v == null ? '—' : fmtBytes(v)),
      },
    ],
    scales: { x: { time: true } },
    axes: [
      { stroke: '#64748b', grid: { stroke: 'rgba(100,116,139,0.15)' }, ticks: { stroke: '#64748b' } },
      {
        stroke: '#64748b',
        grid: { stroke: 'rgba(100,116,139,0.15)' },
        ticks: { stroke: '#64748b' },
        values: (_u: unknown, splits: number[]) => splits.map((v) => fmtBytes(v)),
      },
    ],
    legend: { live: false },
  });

  onMount(() => {
    const node = el;
    if (!node) return;
    let dead = false;
    void (async () => {
      // Dynamic import: uPlot touches canvas/DOM APIs, browser-only.
      const UPlot = (await import('uplot')).default;
      if (dead) return;
      const conf = opts(height);
      conf.width = node.clientWidth || 600;
      // типы uplot 1.6.32 не описывают setSize, в рантайме метод есть
      chart = new UPlot(conf, toSeries(buckets), node) as unknown as Chart;
    })();
    // uPlot ширина задаётся при создании: без ResizeObserver график вылезает из карточки при ресайзе.
    const ro = new ResizeObserver(([e]) => {
      const w = Math.floor(e.contentRect.width);
      if (chart && w > 0) chart.setSize({ width: w, height });
    });
    ro.observe(node);
    return () => {
      dead = true;
      ro.disconnect();
      chart?.destroy();
      chart = null;
    };
  });

  $effect(() => {
    // toSeries читаем всегда: иначе при chart === null (первый рендер) buckets
    // не попадает в зависимости эффекта и график никогда не обновится.
    const data = toSeries(buckets);
    if (chart) chart.setData(data);
  });
</script>

<!-- chart width follows container (ResizeObserver in onMount) -->
<div class="relative" style="min-height:{height}px">
  <div bind:this={el} class="w-full" class:hidden={buckets.length === 0}></div>
  {#if buckets.length === 0}
    <!-- пустые данные → uPlot рисует шкалу от 1970 года, поэтому сам график скрыт -->
    <p class="absolute inset-0 grid place-items-center text-sm text-muted-foreground">
      Нет данных за период
    </p>
  {/if}
</div>
