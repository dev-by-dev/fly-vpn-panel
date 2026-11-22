import { describe, it, expect } from 'vitest';
import { errText } from './errors';
import { setLang } from './i18n.svelte';

describe('errText', () => {
  it('переводит известные коды', () => {
    setLang('ru');
    expect(errText('invalid_credentials')).toBe('Неверный логин или пароль');
    expect(errText('broken_template')).toBe('Шаблон сломан: плейсхолдеры не подставляются');
  });

  it('переводит на текущий язык', () => {
    setLang('en');
    expect(errText('invalid_credentials')).toBe('Wrong login or password');
    setLang('ru');
  });

  it('переводит коды мониторинга', () => {
    setLang('ru');
    expect(errText('journal_unavailable', 'Не удалось загрузить логи (502)')).toBe(
      'Журнал systemd недоступен',
    );
    expect(errText('invalid_source')).toBe('Неизвестный источник логов');
    expect(errText('invalid_range')).toBe('Неизвестный диапазон');
  });

  it('неизвестный код отдаёт fallback, иначе сам код', () => {
    expect(errText('weird_code', 'Своё сообщение')).toBe('Своё сообщение');
    expect(errText('weird_code')).toBe('weird_code');
  });
});