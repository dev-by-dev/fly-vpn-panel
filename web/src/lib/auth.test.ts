import { test, expect, vi, beforeEach } from 'vitest';
import { login, checkSession, authState } from './auth.svelte';

beforeEach(() => {
  authState.loggedIn = false;
  authState.need2fa = false;
  window.location.hash = '';
});

test('login 2fa_required → need2fa state', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValue(new Response('{"error":"2fa_required"}', { status: 401 }));
  vi.stubGlobal('fetch', fetchMock);

  const ok = await login('admin', 'password123');

  expect(ok).toBe(false);
  expect(authState.need2fa).toBe(true);
  expect(authState.loggedIn).toBe(false);
  expect(fetchMock).toHaveBeenCalledTimes(1);
  expect(fetchMock.mock.calls[0][0]).toBe('/api/auth/login');
  const init = fetchMock.mock.calls[0][1];
  expect(init.method).toBe('POST');
  expect(init.credentials).toBe('include');
  expect(init.headers['Content-Type']).toBe('application/json');
  expect(init.body).toBe(JSON.stringify({ username: 'admin', password: 'password123' }));
});

test('checkSession: expired access → refresh → retry → session restored', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response('{"error":"expired"}', { status: 401 }))
    .mockResolvedValueOnce(new Response('{}', { status: 200 }))
    .mockResolvedValueOnce(new Response('{}', { status: 200 }));
  vi.stubGlobal('fetch', fetchMock);

  const ok = await checkSession();

  expect(ok).toBe(true);
  expect(authState.loggedIn).toBe(true);
  expect(fetchMock).toHaveBeenCalledTimes(3);
  expect(fetchMock.mock.calls[0][0]).toBe('/api/settings');
  expect(fetchMock.mock.calls[1][0]).toBe('/api/auth/refresh');
  expect(fetchMock.mock.calls[1][1].method).toBe('POST');
  expect(fetchMock.mock.calls[2][0]).toBe('/api/settings');
});

test('checkSession: hard refresh failure → logged out', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response('{"error":"expired"}', { status: 401 }))
    .mockResolvedValueOnce(new Response('{"error":"invalid"}', { status: 401 }));
  vi.stubGlobal('fetch', fetchMock);

  const ok = await checkSession();

  expect(ok).toBe(false);
  expect(authState.loggedIn).toBe(false);
  expect(fetchMock).toHaveBeenCalledTimes(2); // probe + refresh, no retry
  expect(window.location.hash).toBe('#/login');
});
