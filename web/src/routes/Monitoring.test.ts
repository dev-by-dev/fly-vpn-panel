import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import Monitoring from './Monitoring.svelte';
import { setLang } from '$lib/i18n.svelte';

// uPlot трогает canvas — подменяем класс и считаем построения.
const configs: unknown[] = [];
vi.mock('uplot', () => ({
  default: class {
    static paths = { spline: () => ({}) };
    constructor(conf: unknown) {
      configs.push(conf);
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

const points = [
  {
    ts: '2026-11-22T10:00:00Z',
    rps: 1.5,
    errors_per_s: 0.1,
    avg_ms: 12.5,
    inflight: 2,
    rss: 57671680,
    upstream_healthy: true,
  },
  {
    ts: '2026-11-22T10:00:15Z',
    rps: 2,
    errors_per_s: 0,
    avg_ms: 10,
    inflight: 1,
    rss: 57671680,
    upstream_healthy: true,
  },
];

const summary = {
  requests: 120,
  errors: 3,
  avg_ms: 12.5,
  inflight: 2,
  rss: 57671680,
  upstream_healthy: true,
};

let urls: string[] = [];

function mockFetch(entries: unknown[] = [], caddy: { points: unknown[]; summary: unknown } = { points, summary }) {
  urls = [];
  configs.length = 0;
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    urls.push(url);
    if (url.includes('/api/monitoring/logs')) {
      return new Response(JSON.stringify({ entries }), { status: 200 });
    }
    return new Response(JSON.stringify(caddy), { status: 200 });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

beforeEach(() => {
  setLang('ru');
  vi.stubGlobal('ResizeObserver', RO);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
  setLang('ru');
});

describe('monitoring page', () => {
  test('логи грузятся и меняют источник', async () => {
    mockFetch([
      {
        time: '2026-11-22T10:00:00Z',
        source: 'server',
        level: 'err',
        message: 'boom',
      },
    ]);
    render(Monitoring);
    expect(await screen.findByText('boom')).toBeTruthy();
    expect(urls.some((u) => u.includes('source=server'))).toBe(true);

    await fireEvent.click(screen.getByRole('button', { name: 'xray' }));
    await waitFor(() => expect(urls.some((u) => u.includes('source=xray'))).toBe(true));
  });

  test('caddy показывает сводку и строит графики', async () => {
    mockFetch();
    render(Monitoring);
    await fireEvent.click(screen.getByRole('button', { name: 'Caddy' }));
    expect(await screen.findByText('120')).toBeTruthy();
    expect(screen.getByText('12.5')).toBeTruthy();
    expect(screen.getByText('55 МБ')).toBeTruthy();
    await waitFor(() => expect(configs.length).toBe(2));
    // uPlot берёт width буквально: с нулём график рисуется в ноль пикселей.
    for (const conf of configs) {
      expect((conf as { width: number }).width).toBeGreaterThan(0);
    }
  });

  test('без метрик caddy говорит, что данных нет', async () => {
    mockFetch([], { points: [], summary: {} });
    render(Monitoring);
    await fireEvent.click(screen.getByRole('button', { name: 'Caddy' }));
    expect(await screen.findByText('Данных пока нет')).toBeTruthy();
    expect(configs.length).toBe(0);
  });

  test('английский язык переводит страницу мониторинга', async () => {
    mockFetch([]);
    setLang('en');
    render(Monitoring);
    expect(await screen.findByRole('button', { name: 'Logs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Caddy' })).toBeTruthy();
    expect(screen.queryByText('Логи')).toBeNull();
  });
});