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
let calls: { url: string; method: string }[] = [];

function mockFetch(entries: unknown[] = [], caddy: { points: unknown[]; summary: unknown } = { points, summary }) {
  urls = [];
  calls = [];
  configs.length = 0;
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    urls.push(url);
    calls.push({ url, method: init?.method ?? 'GET' });
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

  test('caddy обновляется сам, без кнопки', async () => {
    vi.useFakeTimers();
    mockFetch();
    render(Monitoring);
    await fireEvent.click(screen.getByRole('button', { name: 'Caddy' }));
    await vi.advanceTimersByTimeAsync(0);
    const caddyCalls = () => urls.filter((u) => u.includes('/api/monitoring/caddy')).length;
    const first = caddyCalls();
    expect(first).toBeGreaterThan(0);
    expect(screen.queryByRole('button', { name: 'Обновить' })).toBeNull();

    await vi.advanceTimersByTimeAsync(15100);
    expect(caddyCalls()).toBeGreaterThan(first);

    vi.useRealTimers();
  });

  test('английский язык переводит страницу мониторинга', async () => {
    mockFetch([]);
    setLang('en');
    render(Monitoring);
    expect(await screen.findByRole('button', { name: 'Logs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Caddy' })).toBeTruthy();
    expect(screen.queryByText('Логи')).toBeNull();
  });

  test('пауза останавливает автообновление', async () => {
    vi.useFakeTimers();
    mockFetch([]);
    render(Monitoring);
    await vi.advanceTimersByTimeAsync(0);

    await fireEvent.click(screen.getByLabelText('Автообновление (5 с)'));
    await vi.advanceTimersByTimeAsync(5100);
    const beforePause = urls.length;
    expect(beforePause).toBeGreaterThan(1);

    await fireEvent.click(screen.getByRole('button', { name: 'Пауза' }));
    await vi.advanceTimersByTimeAsync(5100);
    expect(urls.length).toBe(beforePause);

    await fireEvent.click(screen.getByRole('button', { name: 'Продолжить' }));
    await vi.advanceTimersByTimeAsync(5100);
    expect(urls.length).toBeGreaterThan(beforePause);

    vi.useRealTimers();
  });

  test('очистка в xray обрезает файл и пустеет список', async () => {
    mockFetch([
      { time: '2026-11-22T10:00:00Z', source: 'xray', level: 'warning', message: 'boom' },
    ]);
    render(Monitoring);
    await screen.findByText('boom');
    await fireEvent.click(screen.getByRole('button', { name: 'xray' }));
    await waitFor(() => expect(calls.some((c) => c.url.includes('source=xray'))).toBe(true));
    // даём догрузке применить ответ, иначе очистка гоняется с ней
    await new Promise((r) => setTimeout(r, 0));

    await fireEvent.click(screen.getByRole('button', { name: 'Очистить' }));
    await waitFor(() =>
      expect(
        calls.some((c) => c.method === 'DELETE' && c.url.includes('source=xray')),
      ).toBe(true),
    );
    expect(screen.queryByText('boom')).toBeNull();
  });

  test('после очистки журнала старые записи не возвращаются', async () => {
    const entries = [
      { time: '2020-01-01T00:00:00Z', source: 'server', level: 'err', message: 'old-line' },
    ];
    mockFetch(entries);
    render(Monitoring);
    await screen.findByText('old-line');

    await fireEvent.click(screen.getByRole('button', { name: 'Очистить' }));
    expect(screen.queryByText('old-line')).toBeNull();

    entries.push({ time: '2099-01-01T00:00:00Z', source: 'server', level: 'info', message: 'new-line' });
    await fireEvent.click(screen.getByRole('button', { name: 'Обновить' }));

    expect(await screen.findByText('new-line')).toBeTruthy();
    expect(screen.queryByText('old-line')).toBeNull();
  });

  test('очистка журнала не удаляет записи на сервере', async () => {
    mockFetch([
      { time: '2026-11-22T10:00:00Z', source: 'server', level: 'err', message: 'boom' },
    ]);
    render(Monitoring);
    await screen.findByText('boom');
    await fireEvent.click(screen.getByRole('button', { name: 'Очистить' }));
    expect(screen.queryByText('boom')).toBeNull();
    expect(calls.some((c) => c.method === 'DELETE')).toBe(false);
  });
});