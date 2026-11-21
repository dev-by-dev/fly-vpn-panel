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
  // новый юзер: +30 дней по умолчанию
  let expiry = $state(toLocalInput(user?.expiry ?? new Date(Date.now() + 30 * 864e5).toISOString()));
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
      expiry: new Date(expiry).toISOString(),
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
  <div class="w-full max-w-md rounded-xl border bg-card p-5 shadow-lg">
    <h2 class="text-lg font-semibold">{isEdit ? 'Изменить пользователя' : 'Новый пользователь'}</h2>
    <form class="mt-4 grid gap-4" onsubmit={submit}>
      <div class="grid gap-1.5">
        <Label for="u-name">Имя</Label>
        <Input id="u-name" bind:value={username} disabled={isEdit} placeholder="ivan" />
        {#if isEdit}
          <p class="text-xs text-muted-foreground">Имя изменить нельзя (ключ статистики)</p>
        {/if}
      </div>
      <div class="grid gap-1.5">
        <Label for="u-expiry">Действует до</Label>
        <Input id="u-expiry" type="datetime-local" bind:value={expiry} required />
      </div>
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
      <div class="grid gap-1.5">
        <Label for="u-devices">Лимит устройств (0 — без лимита)</Label>
        <Input id="u-devices" type="number" min="0" step="1" bind:value={devices} />
        <p class="text-xs text-muted-foreground">Подсказка: один роутер = один IP</p>
      </div>
      {#if isEdit}
        <label class="flex items-center gap-2 text-sm">
          <input type="checkbox" bind:checked={manualOff} class="size-4 accent-primary" />
          Отключить вручную
        </label>
      {/if}
      {#if error}
        <p class="text-sm text-red-400">{error}</p>
      {/if}
      <div class="mt-1 flex justify-end gap-2">
        <Button type="button" variant="outline" onclick={onclose}>Отмена</Button>
        <Button type="submit" disabled={saving}>{saving ? 'Сохранение…' : 'Сохранить'}</Button>
      </div>
    </form>
  </div>
</div>
