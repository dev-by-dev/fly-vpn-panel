import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import Users from './Users.svelte';
import { usersIntent } from '$lib/usersIntent.svelte';
import { setLang } from '$lib/i18n.svelte';

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

// Disabled with an empty reason: must still render as disabled.
const carol = {
  ...alice,
  id: 3,
  username: 'carol',
  disabled: true,
  disabled_reason: '',
  quota_bytes: 5368709120,
  used_bytes: 0,
  online_count: 0,
};

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(new Response(JSON.stringify([alice, bob, carol]), { status: 200 }))
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
  // бейджи рендерятся капсом: alice активна, bob отключён по квоте
  expect(screen.getAllByText('АКТИВЕН').length).toBe(1);
  expect(screen.getAllByText('КВОТА').length).toBe(1);
  // carol отключён без причины — не «Активен»
  expect(screen.getAllByText('ОТКЛЮЧЁН').length).toBe(1);
  // трафик used/quota в ячейке с прогресс-баром
  expect(screen.getByText(/1\.5 ГБ \/ 10 ГБ/)).toBeTruthy();
  // онлайн-устройства (alice: 2) — «2» встречается и в карточках статистики
  expect(screen.getAllByText('2').length).toBeGreaterThan(1);
});

test('кнопка «Добавить» открывает диалог создания', async () => {
  render(Users);

  await screen.findByText('alice');
  await fireEvent.click(screen.getByRole('button', { name: 'Добавить' }));

  expect(await screen.findByText('Создать пользователя')).toBeTruthy();
});

test('устройства за одним IP: одна зелёная точка и пометка «тот же IP»', async () => {
  usersIntent.view = 'devices';
  const dev = (ip: string, online: boolean, sameIP: boolean, hwid: string) => ({
    Username: 'alice',
    IP: ip,
    FirstSeen: '2026-01-01T00:00:00Z',
    LastSeen: '2026-11-22T10:00:00Z',
    online,
    same_ip: sameIP,
    HWID: hwid,
    Platform: 'Happ',
    OSVersion: '4.11.2',
    Model: '',
    Country: '',
    ASN: '',
    Provider: '',
    UserAgent: 'Happ/4.11.2',
  });
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify([
          dev('203.0.113.7', true, false, 'mac'),
          dev('203.0.113.7', false, true, 'android'),
          dev('198.51.100.9', false, false, 'away'),
        ]),
        { status: 200 }
      )
    )
  );

  const { container } = render(Users);

  expect(await screen.findByText('тот же IP')).toBeTruthy();
  expect(container.querySelectorAll('.bg-emerald-400').length).toBe(1);
  expect(container.querySelectorAll('.bg-slate-500').length).toBe(1);
  expect(container.querySelectorAll('.bg-red-400').length).toBe(1);
});

test('английский язык переводит таблицу юзеров', async () => {
  setLang('en');
  render(Users);

  expect(await screen.findByText('Users')).toBeTruthy();
  expect(screen.getByText('Username')).toBeTruthy();
  expect(screen.queryByText('Пользователи')).toBeNull();
  setLang('ru');
});
