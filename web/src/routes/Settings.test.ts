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

function mockFetch() {
  return vi.fn((input: string | URL, init?: RequestInit) => {
    const url = String(input);
    if (url === '/api/settings' && (!init || init.method === undefined || init.method === 'GET')) {
      return Promise.resolve(new Response(JSON.stringify({ poll_interval_sec: 30 }), { status: 200 }));
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
