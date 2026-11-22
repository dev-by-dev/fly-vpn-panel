import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import Config from './Config.svelte';
import { setLang } from '$lib/i18n.svelte';

const routing = '{"domainStrategy":"AsIs","rules":[]}';

function json(body: string, status = 200) {
  return Promise.resolve(new Response(body, { status }));
}

let putBody = '';
let routingStatus = 200;
let configBody = '{"inbounds":[]}';

function mockFetch() {
  vi.stubGlobal('fetch', vi.fn((input: string | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? 'GET';
    if (url === '/api/config') return json(configBody);
    if (url === '/api/config/routing' && method === 'GET') {
      return routingStatus === 200 ? json(routing) : json('{"error":"internal"}', routingStatus);
    }
    if (url === '/api/config/routing' && method === 'PUT') {
      putBody = String(init?.body ?? '');
      return json('', 200);
    }
    return json('{}', 404);
  }));
}

beforeEach(() => { putBody = ''; routingStatus = 200; configBody = '{"inbounds":[]}'; mockFetch(); });
afterEach(() => { cleanup(); vi.unstubAllGlobals(); });

test('показывает дефолтный routing в textarea', async () => {
  render(Config);
  const ta = await screen.findByLabelText('Routing JSON');
  await waitFor(() => expect((ta as HTMLTextAreaElement).value).toContain('domainStrategy'));
});

test('Применить отправляет ровно введённый текст', async () => {
  render(Config);
  const ta = (await screen.findByLabelText('Routing JSON')) as HTMLTextAreaElement;
  await waitFor(() => expect(ta.value).toContain('domainStrategy'));
  ta.value = '{"domainStrategy":"IPIfNonMatch","rules":[]}';
  await fireEvent.input(ta);
  await fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
  await waitFor(() => expect(putBody).toBe('{"domainStrategy":"IPIfNonMatch","rules":[]}'));
  await screen.findByText('Применено');
});

test('422 объясняет откат', async () => {
  vi.stubGlobal('fetch', vi.fn((input: string | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/config') return json('{"inbounds":[]}');
    if (init?.method === 'PUT') return json('{"error":"invalid_json"}', 422);
    return json(routing);
  }));
  render(Config);
  await screen.findByLabelText('Routing JSON');
  await fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
  await screen.findByText('Некорректный JSON (422)');
});

test('ошибка загрузки routing не скрывает конфиг', async () => {
  routingStatus = 500;
  render(Config);
  await screen.findByText('Не удалось загрузить routing (500)');
  expect(await screen.findByText(/inbounds/)).toBeTruthy();
});

test('502 после удачной проверки показывается вместо старого сообщения', async () => {
  render(Config);
  const ta = (await screen.findByLabelText('Routing JSON')) as HTMLTextAreaElement;
  await waitFor(() => expect(ta.value).toContain('domainStrategy'));
  await fireEvent.click(screen.getByRole('button', { name: 'Проверить' }));
  await screen.findByText('JSON корректен');
  vi.stubGlobal('fetch', vi.fn((input: string | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/config') return json(configBody);
    if (init?.method === 'PUT') return json('{"error":"xray_down"}', 502);
    return json(routing);
  }));
  await fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
  await screen.findByText('xray недоступен (502)');
});

test('успешное применение перечитывает конфиг', async () => {
  render(Config);
  await screen.findByText(/inbounds/);
  configBody = '{"inbounds":[{"tag":"fresh"}]}';
  await fireEvent.click(screen.getByRole('button', { name: 'Применить' }));
  await screen.findByText(/fresh/);
});
test('английский язык переводит страницу конфига', async () => {
  setLang('en');
  render(Config);
  await screen.findByText(/inbounds/);
  expect(screen.getByText('Config')).toBeTruthy();
  expect(screen.getByRole('button', { name: 'Apply' })).toBeTruthy();
  expect(screen.queryByText('Конфиг')).toBeNull();
  setLang('ru');
});
