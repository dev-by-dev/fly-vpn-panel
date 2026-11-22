import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/svelte';
import Settings from './Settings.svelte';

// qrcode — canvas в happy-dom недоступен, мокаем генерацию QR.
vi.mock('qrcode', () => ({
  toDataURL: vi.fn().mockResolvedValue('data:image/png;base64,QUJDRA=='),
}));

// Anonymized fixtures: fake admin creds, TEST-NET, example.com.
const enroll = {
  secret: 'JBSWY3DPEHPK3PXP',
  otpauth_uri: 'otpauth://totp/flypanel:admin?secret=JBSWY3DPEHPK3PXP&issuer=flypanel',
  backup_codes: Array.from({ length: 10 }, (_, i) => `CODE-${i}`),
};

let totpEnabled = false;
let tokens = [
  { id: 1, name: 'ci', scope: 'read', expires_at: '', created_at: '2026-11-22T10:00:00Z', revoked: false },
];

function mockFetch() {
  return vi.fn((input: string | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/tokens' && (!init || init.method === undefined || init.method === 'GET')) {
      return Promise.resolve(new Response(JSON.stringify({ tokens }), { status: 200 }));
    }
    if (url === '/api/tokens' && init?.method === 'POST') {
      return Promise.resolve(
        new Response(
          JSON.stringify({
            id: 2,
            name: 'ci',
            scope: 'full',
            expires_at: '2026-12-22T00:00:00Z',
            token: 'fp_testtoken',
          }),
          { status: 201 }
        )
      );
    }
    if (/^\/api\/tokens\/\d+$/.test(url) && init?.method === 'DELETE') {
      const id = Number(url.split('/').pop());
      tokens = tokens.map((t) => (t.id === id ? { ...t, revoked: true } : t));
      return Promise.resolve(new Response(null, { status: 204 }));
    }
    if (url === '/api/settings' && (!init || init.method === undefined || init.method === 'GET')) {
      return Promise.resolve(
        new Response(JSON.stringify({ poll_interval_sec: 30, totp_enabled: totpEnabled }), { status: 200 })
      );
    }
    if (url === '/api/auth/2fa/enroll') {
      return Promise.resolve(new Response(JSON.stringify(enroll), { status: 200 }));
    }
    if (url === '/api/auth/2fa/verify') {
      return Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200 }));
    }
    return Promise.resolve(new Response(JSON.stringify({ ok: true }), { status: 200 }));
  });
}

let fetchMock: ReturnType<typeof mockFetch>;

beforeEach(() => {
  totpEnabled = false;
  tokens = [
    { id: 1, name: 'ci', scope: 'read', expires_at: '', created_at: '2026-11-22T10:00:00Z', revoked: false },
  ];
  fetchMock = mockFetch();
  vi.stubGlobal('fetch', fetchMock);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

test('enroll: QR data URL + секрет + 10 бэкап-кодов один раз', async () => {
  render(Settings);

  expect(await screen.findByDisplayValue('30')).toBeTruthy();

  await fireEvent.click(screen.getByRole('button', { name: 'Подключить 2FA' }));

  const img = await screen.findByRole('img');
  expect(img.getAttribute('src')).toBe('data:image/png;base64,QUJDRA==');
  expect(screen.getByText(enroll.secret)).toBeTruthy();
  for (const code of enroll.backup_codes) {
    expect(screen.getByText(code)).toBeTruthy();
  }
  expect(screen.getByText(/Сохрани сейчас/i)).toBeTruthy();
});

test('verify: код отправляется на /api/auth/2fa/verify → включено', async () => {
  render(Settings);

  await screen.findByDisplayValue('30');
  await fireEvent.click(screen.getByRole('button', { name: 'Подключить 2FA' }));
  await screen.findByRole('img');

  const codeInput = screen.getByPlaceholderText('123456');
  await fireEvent.input(codeInput, { target: { value: '123456' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Подтвердить' }));

  await waitFor(() => {
    const call = fetchMock.mock.calls.find(([u]) => String(u) === '/api/auth/2fa/verify');
    expect(call).toBeTruthy();
    expect(JSON.parse(String(call![1]!.body))).toEqual({ code: '123456' });
  });
  expect(await screen.findByText('2FA включена')).toBeTruthy();
});

test('состояние 2FA с сервера: totp_enabled=true → «2FA включена» без enroll', async () => {
  totpEnabled = true;
  render(Settings);

  expect(await screen.findByText('2FA включена')).toBeTruthy();
  expect(screen.queryByRole('button', { name: 'Подключить 2FA' })).toBeNull();
  expect(screen.getByRole('button', { name: 'Отключить' })).toBeTruthy();
});

test('токены: строка из /api/tokens — имя, права, бессрочный', async () => {
  render(Settings);

  const row = (await screen.findByText('ci')).closest('tr');
  expect(row?.textContent).toContain('Только чтение');
  expect(row?.textContent).toContain('Никогда');
});

test('токены: создание отправляет name/scope/expires_in_days и показывает токен', async () => {
  render(Settings);
  await screen.findByText('ci');

  await fireEvent.input(screen.getByLabelText('Название'), { target: { value: 'ci' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Создать' }));

  await waitFor(() => {
    const call = fetchMock.mock.calls.find(
      ([u, i]) => String(u) === '/api/tokens' && i?.method === 'POST'
    );
    expect(call).toBeTruthy();
    expect(JSON.parse(String(call![1]!.body))).toEqual({ name: 'ci', scope: 'full', expires_in_days: 30 });
  });
  expect(await screen.findByText('fp_testtoken')).toBeTruthy();
});

test('токены: «Отозвать» удаляет токен по id', async () => {
  render(Settings);
  await screen.findByText('ci');

  await fireEvent.click(screen.getByRole('button', { name: 'Отозвать ci' }));

  await waitFor(() => {
    const call = fetchMock.mock.calls.find(([u]) => String(u) === '/api/tokens/1');
    expect(call).toBeTruthy();
    expect(call![1]!.method).toBe('DELETE');
  });

  expect(await screen.findByText('Отозван')).toBeTruthy();
  expect(screen.queryByRole('button', { name: /Отозвать/ })).toBeNull();
});

test('токены: отзыв строки не стирает ещё не скопированный токен', async () => {
  render(Settings);
  await screen.findByText('ci');

  await fireEvent.input(screen.getByLabelText('Название'), { target: { value: 'ci' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Создать' }));
  expect(await screen.findByText('fp_testtoken')).toBeTruthy();

  await fireEvent.click(screen.getByRole('button', { name: 'Отозвать ci' }));

  expect(await screen.findByText('Отозван')).toBeTruthy();
  expect(screen.getByText('fp_testtoken')).toBeTruthy();
});

test('токены: без clipboard API копирование говорит об ошибке', async () => {
  vi.stubGlobal('navigator', { ...navigator, clipboard: undefined });
  render(Settings);
  await screen.findByText('ci');

  await fireEvent.input(screen.getByLabelText('Название'), { target: { value: 'ci' } });
  await fireEvent.click(screen.getByRole('button', { name: 'Создать' }));
  await screen.findByText('fp_testtoken');

  await fireEvent.click(screen.getByRole('button', { name: 'Копировать' }));

  expect(await screen.findByText('Не удалось скопировать — выдели вручную')).toBeTruthy();
});
