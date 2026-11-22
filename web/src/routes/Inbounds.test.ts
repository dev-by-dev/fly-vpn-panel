import { render, screen, cleanup } from '@testing-library/svelte';
import { afterEach, beforeEach, expect, test, vi } from 'vitest';
import Inbounds from './Inbounds.svelte';
import InboundForm from '$lib/components/InboundForm.svelte';
import { setLang } from '$lib/i18n.svelte';

beforeEach(() => {
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response('[]', { status: 200 }))),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  setLang('ru');
});

test('английский язык переводит страницу inbounds', async () => {
  setLang('en');
  render(Inbounds);
  expect(await screen.findByText('Port')).toBeTruthy();
  expect(screen.getByText('Enabled')).toBeTruthy();
  expect(screen.queryByText('Порт')).toBeNull();
});

test('английский язык переводит форму inbound', async () => {
  setLang('en');
  render(InboundForm, { inbound: null, onsaved: () => {}, onclose: () => {} });
  expect(screen.getByText('New inbound')).toBeTruthy();
  expect(screen.getByLabelText('Tag')).toBeTruthy();
  expect(screen.queryByText('Новый inbound')).toBeNull();
});