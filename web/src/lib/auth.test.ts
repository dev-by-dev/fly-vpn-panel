import { test, expect, vi, beforeEach } from 'vitest';
import { login, authState } from './auth.svelte';

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
