import { afterEach, beforeEach, describe, expect, test, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import Monitoring from './Monitoring.svelte';
import { setLang } from '$lib/i18n.svelte';

let urls: string[] = [];

function mockFetch(entries: unknown[] = []) {
  urls = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input);
    urls.push(url);
    if (url.includes('/api/monitoring/logs')) {
      return new Response(JSON.stringify({ entries }), { status: 200 });
    }
    return new Response(JSON.stringify({ points: [], summary: {} }), { status: 200 });
  });
  vi.stubGlobal('fetch', fetchMock);
  return fetchMock;
}

beforeEach(() => {
  setLang('ru');
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
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

  test('английский язык переводит страницу мониторинга', async () => {
    mockFetch([]);
    setLang('en');
    render(Monitoring);
    expect(await screen.findByRole('button', { name: 'Logs' })).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Caddy' })).toBeTruthy();
    expect(screen.queryByText('Логи')).toBeNull();
    setLang('ru');
  });
});