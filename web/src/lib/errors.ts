import { t, type Key } from './i18n.svelte';

const known: Record<string, Key> = {
  invalid_credentials: 'errors.invalid_credentials',
  password_too_short: 'errors.password_too_short',
  invalid_token_request: 'errors.invalid_token_request',
  broken_template: 'errors.broken_template',
  invalid_json: 'errors.invalid_json',
  invalid_routing: 'errors.invalid_routing',
  xray_down: 'errors.xray_down',
  not_found: 'errors.not_found',
  unauthorized: 'errors.unauthorized',
  forbidden: 'errors.forbidden',
  bad_request: 'errors.bad_request',
  internal: 'errors.internal',
  journal_unavailable: 'errors.journal_unavailable',
  invalid_source: 'errors.invalid_source',
  invalid_range: 'errors.invalid_range',
};

// Код с бэка переводим на клиенте; незнакомый код показываем как есть.
export function errText(code: string, fallbackText?: string): string {
  const key = known[code];
  if (key) return t(key);
  return fallbackText ?? code;
}