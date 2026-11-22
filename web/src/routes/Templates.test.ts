import { test, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import { tick } from 'svelte';
import Templates from './Templates.svelte';

// Разрешённые плейсхолдеры рендереров: internal/sub/json.go подставляет
// только этот набор (любой лишний {{ → 400 broken_template);
// internal/sub/yaml.go exposes только PROXIES/PROXY_NAMES — per-proxy
// поля (UUID/ADDRESS/…) живут внутри сгенерированной подстановки.
const JSON_ALLOWED = new Set([
  'UUID', 'ADDRESS', 'PORT', 'NETWORK', 'PBK', 'SID', 'SNI', 'FP', 'PATH', 'MODE', 'TAG',
]);
const YAML_ALLOWED = new Set(['PROXIES', 'PROXY_NAMES']);

function placeholders(text: string): string[] {
  return [...text.matchAll(/\{\{([A-Z_]+)\}\}/g)].map((m) => m[1]);
}

function hintText(container: HTMLElement): string {
  return container.querySelector('p code')?.textContent ?? '';
}

beforeEach(() => {
  vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response('', { status: 200 }))));
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.clearAllMocks();
});

test('json hint: ровно плейсхолдеры, которые подставляет json.go', () => {
  const { container } = render(Templates);
  const tokens = placeholders(hintText(container));
  expect(new Set(tokens)).toEqual(JSON_ALLOWED);
});

test.each(['mihomo', 'clash'])('%s hint: только {{PROXIES}}/{{PROXY_NAMES}} из yaml.go', async (fmt) => {
  const { container } = render(Templates);
  await fireEvent.click(screen.getByRole('button', { name: fmt }));
  await tick();
  const tokens = placeholders(hintText(container));
  expect(tokens).toContain('PROXIES');
  expect(tokens).toContain('PROXY_NAMES');
  for (const t of tokens) {
    expect(YAML_ALLOWED.has(t), `${fmt} hint рекламирует ${t}`).toBe(true);
  }
});
