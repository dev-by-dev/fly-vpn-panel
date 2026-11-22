import { describe, it, expect, beforeEach } from 'vitest';
import { i18n, setLang, t } from './i18n.svelte';

describe('i18n', () => {
  beforeEach(() => {
    setLang('ru');
  });

  it('по умолчанию отдаёт русскую строку', () => {
    expect(t('nav.home')).toBe('Главная');
  });

  it('переключает язык и запоминает выбор', () => {
    setLang('en');
    expect(i18n.lang).toBe('en');
    expect(t('nav.home')).toBe('Dashboard');
    expect(localStorage.getItem('lang')).toBe('en');
  });

  it('подставляет переменные в шаблон', () => {
    setLang('en');
    expect(t('common.error')).toBe('Error');
  });

  it('знает все три языка без пропусков', async () => {
    const ru = (await import('./locales/ru')).default;
    const en = (await import('./locales/en')).default;
    const de = (await import('./locales/de')).default;
    const keys = Object.keys(ru).sort();
    expect(Object.keys(en).sort()).toEqual(keys);
    expect(Object.keys(de).sort()).toEqual(keys);
  });
});