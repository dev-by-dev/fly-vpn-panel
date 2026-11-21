import { authState } from './auth.svelte';

const REFRESH_PATH = '/api/auth/refresh';

function jsonInit(opts: RequestInit): RequestInit {
  const init: RequestInit = { ...opts, credentials: 'include' };
  if (opts.body != null) {
    init.headers = {
      'Content-Type': 'application/json',
      ...(opts.headers as Record<string, string> | undefined),
    };
  }
  return init;
}

// On a hard 401 (refresh failed or still 401 after one retry) drop to login.
function authLost(): void {
  authState.loggedIn = false;
  if (window.location.hash !== '#/login') {
    window.location.hash = '#/login';
  }
}

/**
 * Authenticated fetch: credentials included, JSON bodies.
 * On 401 → ONE refresh request → retry original ONCE → else redirect to login.
 * Only the refresh endpoint itself skips the chain (never refresh via refresh).
 */
export async function apiFetch(path: string, opts: RequestInit = {}): Promise<Response> {
  if (path === REFRESH_PATH) {
    return fetch(path, jsonInit(opts));
  }
  const init = jsonInit(opts);
  let res = await fetch(path, init);
  if (res.status !== 401) {
    return res;
  }
  let refreshed = false;
  try {
    const refreshRes = await fetch(REFRESH_PATH, { method: 'POST', credentials: 'include' });
    refreshed = refreshRes.ok;
  } catch {
    refreshed = false;
  }
  if (refreshed) {
    res = await fetch(path, init); // retry once
    if (res.status !== 401) {
      return res;
    }
  }
  authLost();
  return res;
}

// Login/logout use raw fetch above (401 there = 2fa_required or bad creds, never refresh).

// JWT-protected account endpoints (go through apiFetch refresh chain).

export function loginRequest(
  username: string,
  password: string,
  totp?: string
): Promise<Response> {
  const body: Record<string, string> = totp
    ? { username, password, totp }
    : { username, password };
  return fetch('/api/auth/login', {
    method: 'POST',
    credentials: 'include',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

export function logoutRequest(): Promise<Response> {
  return fetch('/api/auth/logout', { method: 'POST', credentials: 'include' });
}

// JWT-protected account endpoints (go through apiFetch refresh chain).

export function enroll2FA(): Promise<Response> {
  return apiFetch('/api/auth/2fa/enroll', { method: 'POST' });
}

export function verify2FA(code: string): Promise<Response> {
  return apiFetch('/api/auth/2fa/verify', { method: 'POST', body: JSON.stringify({ code }) });
}

export function disable2FA(code: string): Promise<Response> {
  return apiFetch('/api/auth/2fa/disable', { method: 'POST', body: JSON.stringify({ code }) });
}

export function changePassword(oldPw: string, newPw: string): Promise<Response> {
  return apiFetch('/api/auth/password', {
    method: 'POST',
    body: JSON.stringify({ old: oldPw, new: newPw }),
  });
}
