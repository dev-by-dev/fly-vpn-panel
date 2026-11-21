<script lang="ts">
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { apiFetch } from '$lib/api';

  type UserRow = {
    id: number;
    username: string;
    expiry: string;
    quota_bytes: number;
    quota_period: string;
    device_limit: number;
    disabled: boolean;
    disabled_reason: string;
  };

  type Props = { user: UserRow | null; onsaved: () => void; onclose: () => void };
  let { user, onsaved, onclose }: Props = $props();

  const isEdit = user !== null;
  const GB = 1024 ** 3;

  // shortcut: квота в ГБ с 3 знаками — погрешность ≤1 МБ против байтового ввода
  let username = $state(user?.username ?? '');
  // новый юзер: +30 дней по умолчанию; дата и время — отдельными полями,
  // чтобы дата открывалась нативным календарём.
  const expiry0 = toLocalInput(user?.expiry ?? new Date(Date.now() + 30 * 864e5).toISOString());
  let expiryDate = $state(expiry0.slice(0, 10));
  let expiryTime = $state(expiry0.slice(11, 16));

  // Календарь-поповер: месяцы, сетка 6×7 с понедельника, быстрые пресеты.
  let calOpen = $state(false);
  let view = $state({ y: Number(expiry0.slice(0, 4)), m: Number(expiry0.slice(5, 7)) - 1 });
  const MONTHS = ['Январь', 'Февраль', 'Март', 'Апрель', 'Май', 'Июнь', 'Июль', 'Август', 'Сентябрь', 'Октябрь', 'Ноябрь', 'Декабрь'];

  function localISO(d: Date): string {
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
  }

  function cells(y: number, m: number): { d: number; cur: boolean; date: string; weekend: boolean }[] {
    const shift = (new Date(y, m, 1).getDay() + 6) % 7;
    return Array.from({ length: 42 }, (_, i) => {
      const dt = new Date(y, m, 1 - shift + i);
      return { d: dt.getDate(), cur: dt.getMonth() === m, date: localISO(dt), weekend: i % 7 >= 5 };
    });
  }

  function shiftMonth(n: number) {
    const dt = new Date(view.y, view.m + n, 1);
    view = { y: dt.getFullYear(), m: dt.getMonth() };
  }

  function applyPreset(months: number) {
    const dt = new Date(`${expiryDate}T${expiryTime || '00:00'}`);
    dt.setMonth(dt.getMonth() + months);
    expiryDate = localISO(dt);
    view = { y: dt.getFullYear(), m: dt.getMonth() };
  }

  function fmtExpiry(): string {
    const [y, m, d] = expiryDate.split('-');
    return `${d}.${m}.${y}, ${expiryTime}`;
  }
  let quotaGB = $state(user ? Number((user.quota_bytes / GB).toFixed(3)) : 0);
  let period = $state(user?.quota_period ?? 'none');
  let devices = $state(user?.device_limit ?? 0);
  // Чекбокс «Отключить вручную»: true только для manual-отключения.
  const manualOff0 = user?.disabled && user.disabled_reason === 'manual';
  let manualOff = $state(manualOff0);
  let error = $state('');
  let saving = $state(false);

  function toLocalInput(iso: string): string {
    const d = new Date(iso);
    const p = (n: number) => String(n).padStart(2, '0');
    return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
  }

  const errorHints: Record<string, string> = {
    duplicate_username: 'Имя уже занято',
    invalid_expiry: 'Невалидная дата',
    username_required: 'Укажите имя',
    invalid_quota_period: 'Невалидный период',
    negative_value: 'Отрицательное значение',
  };

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (!isEdit && username.trim() === '') {
      error = 'Укажите имя';
      return;
    }
    const body: Record<string, unknown> = {
      expiry: new Date(`${expiryDate}T${expiryTime || '00:00'}`).toISOString(),
      quota_bytes: Math.round(quotaGB * GB),
      quota_period: period,
      device_limit: devices,
    };
    if (!isEdit) body.username = username.trim();
    // disabled шлём только если чекбокс меняли: иначе правка чужого поля
    // ре-включит юзера, отключённого квотой/экспирацией.
    else if (manualOff !== manualOff0) body.disabled = manualOff;
    saving = true;
    try {
      const res = await apiFetch(isEdit ? `/api/users/${user!.id}` : '/api/users', {
        method: isEdit ? 'PATCH' : 'POST',
        body: JSON.stringify(body),
      });
      if (res.ok) {
        onsaved();
        return;
      }
      const data = (await res.json().catch(() => ({}))) as { error?: string };
      error = errorHints[data.error ?? ''] ?? `Ошибка: ${data.error ?? res.status}`;
    } catch {
      error = 'Сеть недоступна';
    } finally {
      saving = false;
    }
  }
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="dialog" aria-modal="true">
  <div class="max-h-[90vh] w-full max-w-3xl rounded-xl border bg-card shadow-lg" class:overflow-y-auto={!calOpen} class:overflow-visible={calOpen}>
    <div class="flex items-center justify-between border-b px-5 py-4">
      <h2 class="text-lg font-semibold">{isEdit ? 'Изменить пользователя' : 'Создать пользователя'}</h2>
      <button class="text-muted-foreground hover:text-foreground" onclick={onclose} aria-label="Закрыть">
        ✕
      </button>
    </div>
    <form onsubmit={submit}>
      <div class="grid gap-4 p-5 md:grid-cols-2">
        <!-- левая колонка -->
        <div class="space-y-4">
          <section class="rounded-xl border p-4">
            <div class="flex items-center gap-2 border-b pb-3">
              <span class="grid h-9 w-9 place-items-center rounded-lg bg-sky-500/10 text-sky-400">
                <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2" /><circle cx="9" cy="7" r="4" />
                </svg>
              </span>
              <h3 class="font-medium">Данные пользователя</h3>
            </div>
            <div class="mt-3 grid gap-1.5">
              <Label for="u-name">Юзернейм</Label>
              <Input id="u-name" bind:value={username} disabled={isEdit} placeholder="ivan" />
              <p class="text-xs text-muted-foreground">
                {isEdit ? 'Имя изменить нельзя (ключ статистики)' : 'Юзернейм потом не изменить'}
              </p>
            </div>
          </section>

          <section class="rounded-xl border p-4">
            <div class="flex items-center gap-2 border-b pb-3">
              <span class="grid h-9 w-9 place-items-center rounded-lg bg-amber-500/10 text-amber-400">
                <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <rect x="3" y="4" width="18" height="12" rx="2" /><path d="M8 20h8" />
                </svg>
              </span>
              <h3 class="font-medium">Устройства</h3>
            </div>
            <div class="mt-3 grid gap-1.5">
              <Label for="u-devices">Лимит устройств (0 — без лимита)</Label>
              <Input id="u-devices" type="number" min="0" step="1" bind:value={devices} />
              <p class="text-xs text-muted-foreground">Подсказка: один роутер = один IP</p>
            </div>
          </section>
        </div>

        <!-- правая колонка -->
        <div class="space-y-4">
          <section class="rounded-xl border p-4">
            <div class="flex items-center gap-2 border-b pb-3">
              <span class="grid h-9 w-9 place-items-center rounded-lg bg-violet-500/10 text-violet-400">
                <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M3 17l5-5 4 3 7-8" /><path d="M14 7h6v6" />
                </svg>
              </span>
              <h3 class="font-medium">Трафик и лимиты</h3>
            </div>
            <div class="mt-3 grid gap-3">
              <div class="grid gap-1.5">
                <Label for="u-quota">Квота, ГБ (0 — без лимита)</Label>
                <Input id="u-quota" type="number" min="0" step="0.001" bind:value={quotaGB} />
              </div>
              <div class="grid gap-1.5">
                <Label for="u-period">Сброс квоты</Label>
                <select
                  id="u-period"
                  bind:value={period}
                  class="border-input bg-input/30 h-8 w-full rounded-lg border px-2.5 text-sm outline-none"
                >
                  <option value="none">не сбрасывать</option>
                  <option value="day">день</option>
                  <option value="week">неделя</option>
                  <option value="month">месяц</option>
                </select>
              </div>
            </div>
          </section>

          <section class="rounded-xl border p-4">
            <div class="flex items-center gap-2 border-b pb-3">
              <span class="grid h-9 w-9 place-items-center rounded-lg bg-indigo-500/10 text-indigo-400">
                <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <path d="M12 3l8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7z" />
                </svg>
              </span>
              <h3 class="font-medium">Настройки доступа</h3>
            </div>
            <div class="mt-3 grid gap-1.5">
              <Label for="u-expiry">Действует до</Label>
              <div class="relative">
                <button
                  id="u-expiry"
                  type="button"
                  class="border-input bg-input/30 h-8 w-full rounded-lg border px-2.5 text-left text-sm"
                  onclick={() => (calOpen = !calOpen)}
                >
                  {fmtExpiry()}
                </button>
                {#if calOpen}
                  <div
                    class="absolute right-0 bottom-full z-50 mb-1 flex w-[26rem] max-w-[calc(100vw-2rem)] gap-3 rounded-xl border bg-card p-3 shadow-lg"
                  >
                    <div class="flex w-28 shrink-0 flex-col gap-1 border-r pr-2">
                      {#each [['+1 месяц', 1], ['+3 месяца', 3], ['+1 год', 12]] as [label, n] (label)}
                        <button
                          type="button"
                          class="rounded px-2 py-1 text-left text-sm hover:bg-muted"
                          onclick={() => applyPreset(n as number)}>{label}</button
                        >
                      {/each}
                      <button
                        type="button"
                        class="rounded px-2 py-1 text-left text-sm hover:bg-muted"
                        onclick={() => {
                          expiryDate = '2099-12-31';
                          view = { y: 2099, m: 11 };
                        }}>до 2099 года</button
                      >
                    </div>
                    <div class="min-w-0 flex-1">
                      <div class="flex items-center justify-between">
                        <button type="button" class="px-1 text-lg leading-none" onclick={() => shiftMonth(-1)}>‹</button>
                        <span class="text-sm font-medium">{MONTHS[view.m]} {view.y}</span>
                        <button type="button" class="px-1 text-lg leading-none" onclick={() => shiftMonth(1)}>›</button>
                      </div>
                      <div class="mt-2 grid grid-cols-7 gap-1 text-center text-xs text-muted-foreground">
                        {#each ['Пн', 'Вт', 'Ср', 'Чт', 'Пт', 'Сб', 'Вс'] as w (w)}<span>{w}</span>{/each}
                      </div>
                      <div class="mt-1 grid grid-cols-7 gap-1">
                        {#each cells(view.y, view.m) as c (c.date)}
                          <button
                            type="button"
                            class="h-7 rounded text-xs {c.date === expiryDate
                              ? 'bg-primary text-primary-foreground'
                              : c.cur
                                ? 'hover:bg-muted'
                                : 'text-muted-foreground'} {c.weekend && c.cur && c.date !== expiryDate
                              ? 'text-red-400'
                              : ''}"
                            onclick={() => (expiryDate = c.date)}>{c.d}</button
                          >
                        {/each}
                      </div>
                      <div class="mt-2 flex items-center justify-between border-t pt-2">
                        <input
                          type="time"
                          bind:value={expiryTime}
                          class="border-input bg-input/30 h-8 rounded-md border px-2 text-sm"
                        />
                        <button
                          type="button"
                          class="rounded-md bg-primary px-3 py-1 text-sm text-primary-foreground"
                          onclick={() => (calOpen = false)}>✓</button
                        >
                      </div>
                    </div>
                  </div>
                {/if}
              </div>
            </div>
            {#if isEdit}
              <label class="mt-3 flex items-center gap-2 text-sm">
                <input type="checkbox" bind:checked={manualOff} class="size-4 accent-primary" />
                Отключить вручную
              </label>
            {/if}
          </section>
        </div>
      </div>

      {#if error}
        <p class="px-5 pb-2 text-sm text-red-400">{error}</p>
      {/if}
      <div class="flex justify-end gap-2 border-t px-5 py-4">
        <Button type="button" variant="outline" onclick={onclose}>Отмена</Button>
        <Button type="submit" disabled={saving}>{saving ? 'Сохранение…' : isEdit ? 'Сохранить' : 'Создать'}</Button>
      </div>
    </form>
  </div>
</div>
