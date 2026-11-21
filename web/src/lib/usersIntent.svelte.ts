// Намерение из выпадающего меню вкладки «Юзеры»: страница применяет его
// один раз и сбрасывает.
export const usersIntent = $state<{ view: 'devices' | 'list' | null }>({ view: null });
