import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import Users from './Users.svelte';

// Anonymized fixtures (public repo): fake uuids/tokens, RFC3339 timestamps.
const alice = {
  id: 1,
  username: 'alice',
  uuid: '11111111-1111-4111-8111-111111111111',
  sub_token: 'AAAAAAAAAAAAAAAA',
  expiry: '2027-01-01T00:00:00Z',
  quota_bytes: 10737418240,
  quota_period: 'month',
  quota_reset_at: '2026-11-01T00:00:00Z',
  device_limit: 3,
  disabled: false,
  disabled_reason: '',
  created_at: '2026-01-01T00:00:00Z',
  used_bytes: 1610612736,
  online_count: 2,
};

const bob = {
  ...alice,
  id: 2,
  username: 'bob',
  disabled: true,
  disabled_reason: 'quota',
  used_bytes: 10737418240,
  online_count: 0,
};

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response(JSON.stringify([alice, bob]), { status: 200 }))
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test('рендер таблицы: 2 юзера, бейджи «Активен» и «Квота»', async () => {
  render(Users);

  expect(await screen.findByText('alice')).toBeTruthy();
  expect(screen.getByText('bob')).toBeTruthy();
  // alice активна, bob отключён по квоте
  expect(screen.getByText('Активен')).toBeTruthy();
  expect(screen.getByText('Квота')).toBeTruthy();
  // трафик used/quota
  expect(screen.getByText('1.5 ГБ / 10 ГБ')).toBeTruthy();
  // онлайн-устройства
  expect(screen.getByText('2')).toBeTruthy();
});

test('кнопка «Добавить» открывает диалог создания', async () => {
  render(Users);

  await screen.findByText('alice');
  await fireEvent.click(screen.getByRole('button', { name: 'Добавить' }));

  expect(await screen.findByText('Новый пользователь')).toBeTruthy();
});
