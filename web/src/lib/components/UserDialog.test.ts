import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import UserDialog from './UserDialog.svelte';

// Anonymized fixture: quota-disabled user (checkbox starts unchecked).
const quotaUser = {
  id: 7,
  username: 'carol',
  expiry: '2027-01-01T00:00:00Z',
  quota_bytes: 10737418240,
  quota_period: 'month',
  device_limit: 0,
  disabled: true,
  disabled_reason: 'quota',
};

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify({ ok: true }), { status: 200 }));
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function submit() {
  const form = document.querySelector('form');
  if (!form) throw new Error('form not found');
  fireEvent.submit(form);
}

function patchBody(): Record<string, unknown> {
  const call = fetchMock.mock.calls.find(([u]) => String(u) === '/api/users/7');
  if (!call) throw new Error('PATCH /api/users/7 not called');
  return JSON.parse(String(call[1].body));
}

test('квота-отключённый юзер: чекбокс не тронут → disabled не отправляется', async () => {
  render(UserDialog, {
    props: { user: { ...quotaUser }, onsaved: () => {}, onclose: () => {} },
  });

  submit();
  await waitFor(() => expect(fetchMock).toHaveBeenCalled());

  const body = patchBody();
  expect('disabled' in body).toBe(false);
  expect(body.quota_bytes).toBe(10737418240);
});

test('чекбокс переключен → disabled отправляется', async () => {
  render(UserDialog, {
    props: { user: { ...quotaUser }, onsaved: () => {}, onclose: () => {} },
  });

  await fireEvent.click(screen.getByRole('checkbox'));
  submit();
  await waitFor(() => expect(fetchMock).toHaveBeenCalled());

  const body = patchBody();
  expect(body.disabled).toBe(true);
});
