import { apiFetch, loginRequest, logoutRequest } from './api';

// Runes store (Svelte 5): reassign fields, or reassign via $state mutation.
export const authState = $state({
  loggedIn: false,
  need2fa: false,
});

// Session probe: GET /api/settings via apiFetch so an expired access token
// is refreshed (15м access + 7д refresh design); hard failure → login screen.
export async function checkSession(): Promise<boolean> {
  try {
    const res = await apiFetch('/api/settings');
    authState.loggedIn = res.ok;
  } catch {
    authState.loggedIn = false;
  }
  return authState.loggedIn;
}

// Login: on 2fa_required (401 {"error":"2fa_required"}) set need2fa so the
// Login form can show the code field and resubmit with totp.
export async function login(
  username: string,
  password: string,
  totp?: string
): Promise<boolean> {
  let res: Response;
  try {
    res = await loginRequest(username, password, totp);
  } catch {
    return false;
  }
  if (res.ok) {
    authState.loggedIn = true;
    authState.need2fa = false;
    window.location.hash = '#/';
    return true;
  }
  if (res.status === 401) {
    const data = (await res.clone().json().catch(() => ({}))) as { error?: string };
    if (data.error === '2fa_required') {
      authState.need2fa = true;
      return false;
    }
  }
  // wrong creds / wrong totp: keep need2fa as-is so the code field persists
  return false;
}

export async function logout(): Promise<void> {
  try {
    await logoutRequest();
  } catch {
    // best effort: clear local state regardless
  }
  authState.loggedIn = false;
  authState.need2fa = false;
  window.location.hash = '#/login';
}
