<script lang="ts">
  import { onMount } from 'svelte';
  // Без CSS uPlot не позиционирует ось/легенду: они наезжают на график и соседние блоки.
  import 'uplot/dist/uPlot.min.css';
  import { toSeries, fmtBytes, fmtRate, type Bucket } from '$lib/traffic';
  import { i18n, t } from '$lib/i18n.svelte';

  let {
    buckets = [],
    height = 260,
    rate = false,
  }: { buckets?: Bucket[]; height?: number; rate?: boolean } = $props();

  // rate=true: данные уже в бит/с, оси и подсказки — fmtRate.
  const fmt = (v: number) => (rate ? fmtRate(v) : fmtBytes(v));

  // uPlot рисует легенду сам: после смены языка обновляем подписи вручную.
  $effect(() => {
    void i18n.lang;
    if (!chart) return;
    chart.series[1].label = t('chart.up');
    chart.series[2].label = t('chart.down');
  });

  let el = $state<HTMLDivElement | null>(null);
  // chart — non-reactive: mutated imperatively, $effect pushes data into it.
  // Structural type: пакет uplot отдаёт класс через `export =`, InstanceType на нём не выводится.
  type Chart = {
    setData(d: number[][]): void;
    setSize(s: { width: number; height: number }): void;
    destroy(): void;
    series: Array<{ label: string }>;
  };
  let chart: Chart | null = null;

  const opts = (h: number) => ({
    width: 0, // set after mount from el.clientWidth
    height: h,
    class: 'uplot-dark',
    series: [
      {},
      {
        label: t('chart.up'),
        stroke: '#34d399',
        fill: 'rgba(52,211,153,0.15)',
        value: (u: unknown, v: number) => (v == null ? '—' : fmt(v)),
      },
      {
        label: t('chart.down'),
        stroke: '#60a5fa',
        fill: 'rgba(96,165,250,0.15)',
        value: (u: unknown, v: number) => (v == null ? '—' : fmt(v)),
      },
    ],
    scales: { x: { time: true } },
    axes: [
      {
        stroke: '#64748b',
        grid: { stroke: 'rgba(100,116,139,0.15)' },
        ticks: { stroke: '#64748b' },
        // 24-часовой формат: дефолт uPlot печатает «8pm». Короткий диапазон — ЧЧ:ММ, длинный — ДД.ММ ЧЧ:ММ.
        values: (_u: unknown, splits: number[]) => {
          const span = splits.length > 1 ? splits[splits.length - 1] - splits[0] : 0;
          return splits.map((s) => {
            const d = new Date(s * 1000);
            const time = d.toLocaleTimeString('ru-RU', { hour: '2-digit', minute: '2-digit', hour12: false });
            return span > 86400
              ? `${d.toLocaleDateString('ru-RU', { day: '2-digit', month: '2-digit' })} ${time}`
              : time;
          });
        },
      },
      {
        stroke: '#64748b',
        grid: { stroke: 'rgba(100,116,139,0.15)' },
        ticks: { stroke: '#64748b' },
        // Дефолтной ширины не хватает для «4.2 Мбит/с» — подписи обрезались.
        size: 80,
        values: (_u: unknown, splits: number[]) => splits.map((v) => fmt(v)),
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
      // Плавные линии: monotone cubic из uPlot (без выбросов за пределы данных).
      // Типы `export = uPlot` не отдают статик paths на default-импорте, в рантайме он есть.
      const spline = (UPlot as unknown as { paths: { spline(): unknown } }).paths.spline();
      (conf.series[1] as Record<string, unknown>).paths = spline;
      (conf.series[2] as Record<string, unknown>).paths = spline;
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
      {t('chart.empty')}
    </p>
  {/if}
</div>
