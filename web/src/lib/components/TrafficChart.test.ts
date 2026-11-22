import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, cleanup, waitFor } from '@testing-library/svelte';
import { setLang } from '$lib/i18n.svelte';

// uPlot трогает canvas/DOM — подменяем класс и запоминаем конфиги.
const configs: Array<{ series: Array<{ label?: string }> }> = [];

vi.mock('uplot', () => ({
  default: class {
    static paths = { spline: () => ({}) };
    series: Array<{ label?: string }>;
    constructor(conf: { series: Array<{ label?: string }> }) {
      configs.push(conf);
      this.series = conf.series;
    }
    setData() {}
    setSize() {}
    destroy() {}
  },
}));

class RO {
  observe() {}
  disconnect() {}
}

beforeEach(() => {
  configs.length = 0;
  vi.stubGlobal('ResizeObserver', RO);
  setLang('ru');
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  setLang('ru');
});

test('легенда графика перерисовывается на новом языке', async () => {
  const { default: TrafficChart } = await import('./TrafficChart.svelte');
  render(TrafficChart, { buckets: [], height: 200 });

  await waitFor(() => expect(configs.length).toBe(1));
  expect(configs[0].series[1].label).toBe('Отдача');
  expect(configs[0].series[2].label).toBe('Загрузка');

  setLang('en');

  await waitFor(() => expect(configs.length).toBe(2));
  expect(configs[1].series[1].label).toBe('Upload');
  expect(configs[1].series[2].label).toBe('Download');
});