import ru from './locales/ru';
import en from './locales/en';
import de from './locales/de';

export type Lang = 'ru' | 'en' | 'de';
export type Key = keyof typeof ru;

export const langs: Lang[] = ['ru', 'en', 'de'];

const dicts: Record<Lang, Record<Key, string>> = { ru, en, de };

function isLang(v: string | null): v is Lang {
  return v === 'ru' || v === 'en' || v === 'de';
}

// Язык браузера решает только при первом визите: дальше слово за переключателем.
function initialLang(): Lang {
  const saved = localStorage.getItem('lang');
  if (isLang(saved)) return saved;
  const nav = (navigator.language ?? '').toLowerCase();
  if (nav.startsWith('ru')) return 'ru';
  if (nav.startsWith('de')) return 'de';
  return 'en';
}

export const i18n = $state({ lang: initialLang() });

// Тег для toLocaleString/Intl: даты и месяцы форматирует сам браузер.
const dateTags: Record<Lang, string> = { ru: 'ru-RU', en: 'en-GB', de: 'de-DE' };

export function dateTag(): string {
  return dateTags[i18n.lang];
}

export function setLang(lang: Lang): void {
  i18n.lang = lang;
  localStorage.setItem('lang', lang);
}

export function t(key: Key, vars?: Record<string, string | number>): string {
  let s = dicts[i18n.lang][key] ?? ru[key];
  if (vars) {
    for (const [k, v] of Object.entries(vars)) s = s.split(`{${k}}`).join(String(v));
  }
  return s;
}