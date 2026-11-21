// Намерение из выпадающего меню вкладки «Inbounds» в шапке: страница
// применяет его один раз и сбрасывает.
export const inboundsIntent = $state<{ action: 'create' | 'metrics' | null }>({ action: null });
