import { describe, it, expect, afterEach, beforeEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import Login from './Login.svelte';
import { setLang } from '$lib/i18n.svelte';

beforeEach(() => setLang('ru'));
afterEach(cleanup);

describe('Login i18n', () => {
  it('по умолчанию русский', () => {
    render(Login);
    expect(screen.getByText('Вход в панель управления')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Войти' })).toBeTruthy();
  });

  it('переключается на английский', () => {
    setLang('en');
    render(Login);
    expect(screen.getByText('Sign in to the panel')).toBeTruthy();
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeTruthy();
    expect(screen.queryByText('Войти')).toBeNull();
  });
});