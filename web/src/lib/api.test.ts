import { test, expect, vi, beforeEach } from 'vitest';
import { apiFetch } from './api';
import { authState } from './auth.svelte';

beforeEach(() => {
  authState.loggedIn = false;
  authState.need2fa = false;
  window.location.hash = '';
});

test('401 → refresh → retry once', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response('{"error":"expired"}', { status: 401 }))
    .mockResolvedValueOnce(new Response('{}', { status: 200 }))
    .mockResolvedValueOnce(new Response('{"items":[]}', { status: 200 }));
  vi.stubGlobal('fetch', fetchMock);

  const res = await apiFetch('/api/users');

  expect(res.status).toBe(200);
  expect(fetchMock).toHaveBeenCalledTimes(3);
  expect(fetchMock.mock.calls[0][0]).toBe('/api/users');
  expect(fetchMock.mock.calls[0][1].credentials).toBe('include');
  expect(fetchMock.mock.calls[1][0]).toBe('/api/auth/refresh');
  expect(fetchMock.mock.calls[1][1].method).toBe('POST');
  expect(fetchMock.mock.calls[1][1].credentials).toBe('include');
  expect(fetchMock.mock.calls[2][0]).toBe('/api/users');
});

test('refresh fail → logout redirect', async () => {
  const fetchMock = vi
    .fn()
    .mockResolvedValueOnce(new Response('{"error":"expired"}', { status: 401 }))
    .mockResolvedValueOnce(new Response('{"error":"invalid"}', { status: 401 }));
  vi.stubGlobal('fetch', fetchMock);

  authState.loggedIn = true;
  const res = await apiFetch('/api/users');

  expect(res.status).toBe(401);
  expect(fetchMock).toHaveBeenCalledTimes(2); // original + refresh, no retry
  expect(authState.loggedIn).toBe(false);
  expect(window.location.hash).toBe('#/login');
});
