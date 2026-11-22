import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/svelte';
import App from './App.svelte';
import { authState } from '$lib/auth.svelte';
import { setLang } from '$lib/i18n.svelte';

beforeEach(() => {
  setLang('ru');
  authState.loggedIn = true;
  vi.stubGlobal(
    'fetch',
    vi.fn(() => Promise.resolve(new Response('[]', { status: 200 }))),
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe('шапка', () => {
  it('переключает язык интерфейса', async () => {
    render(App);
    expect(await screen.findByText('Настройки')).toBeTruthy();

    await fireEvent.click(screen.getByRole('button', { name: 'EN' }));

    expect(await screen.findByText('Settings')).toBeTruthy();
    expect(screen.queryByText('Настройки')).toBeNull();
  });
});