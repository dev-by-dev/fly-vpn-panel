<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch, enroll2FA, verify2FA, disable2FA, changePassword } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { toDataURL as qrDataUrl } from 'qrcode';

  type Enroll = { secret: string; otpauth_uri: string; backup_codes: string[] };

  // poll interval
  let interval = $state('30');
  let intervalMsg = $state('');
  let intervalOk = $state(false);

  // password
  let oldPw = $state('');
  let newPw = $state('');
  let pwMsg = $state('');
  let pwOk = $state(false);

  // 2FA
  let enroll = $state<Enroll | null>(null);
  let qrSrc = $state('');
  let code = $state('');
  let disableCode = $state('');
  let twofaMsg = $state('');
  let twofaOk = $state(false);
  let enabled = $state(false);

  // API tokens
  type Token = { id: number; name: string; scope: string; expires_at: string; created_at: string; revoked: boolean };
  let tokens = $state<Token[]>([]);
  let tokenName = $state('');
  let tokenScope = $state('full');
  let tokenDays = $state('30');
  let newToken = $state('');
  let tokenMsg = $state('');
  let copyMsg = $state('');

  onMount(async () => {
    void loadTokens();
    try {
      const res = await apiFetch('/api/settings');
      if (res.ok) {
        const data = (await res.json()) as { poll_interval_sec: number; totp_enabled?: boolean };
        interval = String(data.poll_interval_sec);
        // 2FA state comes from the server, not session-local memory,
        // so a reload shows the real enabled state.
        enabled = data.totp_enabled === true;
      }
    } catch {
      // не критично: останется значение по умолчанию
    }
  });

  async function saveInterval() {
    intervalMsg = '';
    const v = Number(interval);
    if (!Number.isInteger(v) || v < 5 || v > 300) {
      intervalOk = false;
      intervalMsg = 'Значение 5–300 секунд';
      return;
    }
    try {
      const res = await apiFetch('/api/settings', {
        method: 'PUT',
        body: JSON.stringify({ poll_interval_sec: v }),
      });
      intervalOk = res.ok;
      intervalMsg = res.ok ? 'Сохранено' : `Ошибка (${res.status})`;
    } catch {
      intervalOk = false;
      intervalMsg = 'Сеть недоступна';
    }
  }

  async function savePassword() {
    pwMsg = '';
    try {
      const res = await changePassword(oldPw, newPw);
      if (res.ok) {
        pwOk = true;
        pwMsg = 'Пароль изменён, все сессии отозваны';
        oldPw = '';
        newPw = '';
      } else {
        const data = (await res.json().catch(() => ({}))) as { error?: string };
        pwOk = false;
        pwMsg =
          data.error === 'invalid_credentials'
            ? 'Старый пароль неверен'
            : data.error === 'password_too_short'
              ? 'Новый пароль: минимум 12 символов'
              : `Ошибка (${res.status})`;
      }
    } catch {
      pwOk = false;
      pwMsg = 'Сеть недоступна';
    }
  }

  async function startEnroll() {
    twofaMsg = '';
    try {
      const res = await enroll2FA();
      if (!res.ok) {
        twofaOk = false;
        twofaMsg = `Ошибка (${res.status})`;
        return;
      }
      enroll = (await res.json()) as Enroll;
      qrSrc = await qrDataUrl(enroll.otpauth_uri);
    } catch {
      twofaOk = false;
      twofaMsg = 'Сеть недоступна';
    }
  }

  async function confirmEnroll() {
    if (!code) return;
    twofaMsg = '';
    try {
      const res = await verify2FA(code);
      if (res.ok) {
        enabled = true;
        enroll = null;
        qrSrc = '';
        code = '';
        twofaOk = true;
        twofaMsg = '2FA включена';
      } else {
        twofaOk = false;
        twofaMsg = res.status === 401 ? 'Неверный код' : `Ошибка (${res.status})`;
      }
    } catch {
      twofaOk = false;
      twofaMsg = 'Сеть недоступна';
    }
  }

  async function turnOff() {
    if (!disableCode) return;
    twofaMsg = '';
    try {
      const res = await disable2FA(disableCode);
      if (res.ok) {
        enabled = false;
        disableCode = '';
        twofaOk = true;
        twofaMsg = '2FA отключена';
      } else {
        twofaOk = false;
        twofaMsg = res.status === 401 ? 'Неверный код' : `Ошибка (${res.status})`;
      }
    } catch {
      twofaOk = false;
      twofaMsg = 'Сеть недоступна';
    }
  }

  async function loadTokens() {
    try {
      const res = await apiFetch('/api/tokens');
      if (res.ok) tokens = ((await res.json()) as { tokens: Token[] }).tokens;
    } catch {
      // список не критичен для остальных настроек
    }
  }

  async function createToken() {
    tokenMsg = '';
    newToken = '';
    try {
      const res = await apiFetch('/api/tokens', {
        method: 'POST',
        body: JSON.stringify({ name: tokenName, scope: tokenScope, expires_in_days: Number(tokenDays) }),
      });
      const data = (await res.json().catch(() => ({}))) as { token?: string; error?: string };
      if (res.ok && data.token) {
        newToken = data.token;
        copyMsg = '';
        tokenName = '';
        await loadTokens();
      } else {
        tokenMsg = data.error === 'invalid_token_request' ? 'Укажите название (до 64 символов)' : `Ошибка (${res.status})`;
      }
    } catch {
      tokenMsg = 'Сеть недоступна';
    }
  }

  async function revokeToken(id: number) {
    tokenMsg = '';
    try {
      const res = await apiFetch(`/api/tokens/${id}`, { method: 'DELETE' });
      if (res.ok) await loadTokens();
      else tokenMsg = `Ошибка (${res.status})`;
    } catch {
      tokenMsg = 'Сеть недоступна';
    }
  }

  async function copyToken() {
    copyMsg = '';
    if (!navigator.clipboard) {
      copyMsg = 'Не удалось скопировать — выдели вручную';
      return;
    }
    try {
      await navigator.clipboard.writeText(newToken);
    } catch {
      copyMsg = 'Не удалось скопировать — выдели вручную';
    }
  }

  function scopeLabel(scope: string) {
    return scope === 'read' ? 'Только чтение' : 'Полный доступ';
  }

  function expiryLabel(t: Token) {
    return t.expires_at ? new Date(t.expires_at).toLocaleDateString('ru-RU') : 'Никогда';
  }
</script>

<h1 class="text-2xl font-semibold">Настройки</h1>

<div class="mt-4 grid max-w-2xl grid-cols-1 gap-6">
  <!-- API tokens -->
  <section class="rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
    <div class="flex items-start gap-3">
      <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-sky-500/15 text-sky-300">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4" aria-hidden="true">
          <circle cx="8" cy="15" r="4" /><path d="M10.8 12.2 20 3M16 7l3 3M13 10l3 3" />
        </svg>
      </div>
      <div>
        <h2 class="text-base font-medium">Токены API</h2>
        <p class="text-sm text-muted-foreground">Управление токенами доступа к API.</p>
      </div>
    </div>

    <table class="mt-4 w-full text-sm">
      <thead class="text-xs uppercase text-muted-foreground">
        <tr>
          <th class="pb-1 text-left font-normal">Название</th>
          <th class="pb-1 text-left font-normal">Права</th>
          <th class="pb-1 text-left font-normal">Истекает</th>
          <th></th>
        </tr>
      </thead>
      <tbody>
        {#each tokens as t (t.id)}
          <tr class="border-t border-sky-500/10" class:text-muted-foreground={t.revoked}>
            <td class="py-2">{t.name}</td>
            <td class="text-muted-foreground">{scopeLabel(t.scope)}</td>
            <td class="text-muted-foreground">{expiryLabel(t)}</td>
            <td class="text-right">
              {#if t.revoked}
                <span class="text-xs text-muted-foreground">Отозван</span>
              {:else}
                <button
                  class="text-xs text-muted-foreground hover:text-red-400"
                  aria-label="Отозвать {t.name}"
                  onclick={() => revokeToken(t.id)}>Отозвать</button
                >
              {/if}
            </td>
          </tr>
        {:else}
          <tr class="border-t border-sky-500/10">
            <td colspan="4" class="py-2 text-muted-foreground">Токенов пока нет.</td>
          </tr>
        {/each}
      </tbody>
    </table>

    <div class="mt-4 grid grid-cols-3 gap-3">
      <div>
        <Label for="token-name">Название</Label>
        <Input id="token-name" bind:value={tokenName} placeholder="ci-monitoring" />
      </div>
      <div>
        <Label for="token-scope">Права</Label>
        <select
          id="token-scope"
          bind:value={tokenScope}
          class="mt-1 h-9 w-full rounded-md border bg-transparent px-3 text-sm"> <option value="full">Полный доступ</option>
          <option value="read">Только чтение</option>
        </select>
      </div>
      <div>
        <Label for="token-days">Срок</Label>
        <select
          id="token-days"
          bind:value={tokenDays}
          class="mt-1 h-9 w-full rounded-md border bg-transparent px-3 text-sm"> <option value="30">30 дней</option>
          <option value="90">90 дней</option>
          <option value="0">Бессрочно</option>
        </select>
      </div>
    </div>
    <div class="mt-3 flex items-center gap-3">
      <Button onclick={createToken}>Создать</Button>
      {#if tokenMsg}
        <span class="text-sm text-red-400">{tokenMsg}</span>
      {/if}
    </div>

    {#if newToken}
      <div class="mt-3 rounded-lg border border-emerald-500/30 bg-emerald-500/5 p-3">
        <p class="text-xs text-amber-400">Токен показывается один раз — сохрани сейчас.</p>
        <code class="mt-1 block break-all font-mono text-sm">{newToken}</code>
        <Button variant="outline" class="mt-2" onclick={copyToken}>Копировать</Button>
        {#if copyMsg}
          <p class="mt-1 text-xs text-amber-400">{copyMsg}</p>
        {/if}
      </div>
    {/if}
  </section>

  <!-- Password -->
  <section class="rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
    <div class="flex items-start gap-3">
      <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-sky-500/15 text-sky-300">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4" aria-hidden="true">
          <rect x="4" y="10" width="16" height="11" rx="2" /><path d="M8 10V7a4 4 0 0 1 8 0v3" />
        </svg>
      </div>
      <div>
        <h2 class="text-base font-medium">Смена пароля</h2>
        <p class="text-sm text-muted-foreground">Пароль администратора панели.</p>
      </div>
    </div>
    <div class="mt-3 grid grid-cols-2 gap-3">
      <div>
        <Label for="old-pw">Старый пароль</Label>
        <Input id="old-pw" type="password" bind:value={oldPw} autocomplete="current-password" />
      </div>
      <div>
        <Label for="new-pw">Новый пароль (12+ символов)</Label>
        <Input id="new-pw" type="password" bind:value={newPw} autocomplete="new-password" />
      </div>
    </div>
    <div class="mt-3 flex items-center gap-3">
      <Button onclick={savePassword}>Сменить пароль</Button>
      {#if pwMsg}
        <span class="text-sm {pwOk ? 'text-emerald-400' : 'text-red-400'}">{pwMsg}</span>
      {/if}
    </div>
  </section>

  <!-- Poll interval -->
  <section class="rounded-xl border border-emerald-500/20 bg-emerald-500/5 p-4">
    <div class="flex items-start gap-3">
      <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-emerald-500/15 text-emerald-300">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4" aria-hidden="true">
          <circle cx="12" cy="12" r="9" /><path d="M12 7v5l3 2" />
        </svg>
      </div>
      <div>
        <h2 class="text-base font-medium">Интервал поллинга</h2>
        <p class="text-sm text-muted-foreground">Как часто панель опрашивает xray.</p>
      </div>
    </div>
    <div class="mt-3 flex items-end gap-3">
      <div>
        <Label for="poll-interval">Секунды (5–300)</Label>
        <Input id="poll-interval" type="number" min="5" max="300" bind:value={interval} class="w-28" />
      </div>
      <Button variant="outline" onclick={saveInterval}>Сохранить</Button>
      {#if intervalMsg}
        <span class="text-sm {intervalOk ? 'text-emerald-400' : 'text-red-400'}">{intervalMsg}</span>
      {/if}
    </div>
  </section>

  <!-- 2FA -->
  <section class="rounded-xl border border-violet-500/20 bg-violet-500/5 p-4">
    <div class="flex items-start gap-3">
      <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-violet-500/15 text-violet-300">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="h-4 w-4" aria-hidden="true">
          <path d="M12 3l8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7z" /><path d="M9 12l2 2 4-4" />
        </svg>
      </div>
      <div>
        <h2 class="text-base font-medium">Двухфакторная аутентификация</h2>
        <p class="text-sm text-muted-foreground">Код из приложения при входе.</p>
      </div>
    </div>

    {#if enroll}
      <div class="mt-3">
        <p class="text-sm text-amber-400">Сохрани сейчас — бэкап-коды показываются только один раз.</p>
        <div class="mt-3 flex flex-wrap items-start gap-4">
          {#if qrSrc}
            <img src={qrSrc} alt="QR код 2FA" class="h-40 w-40 rounded-lg border" />
          {/if}
          <div>
            <div class="text-xs text-muted-foreground">Секрет (ввод вручную):</div>
            <code class="block break-all rounded bg-muted px-2 py-1 font-mono text-sm">{enroll.secret}</code>
            <div class="mt-3 text-xs text-muted-foreground">Бэкап-коды (10):</div>
            <div class="mt-1 grid grid-cols-2 gap-x-4 font-mono text-sm">
              {#each enroll.backup_codes as c (c)}
                <span>{c}</span>
              {/each}
            </div>
          </div>
        </div>
        <div class="mt-4 flex items-end gap-3">
          <div>
            <Label for="totp-code">Код из приложения</Label>
            <Input id="totp-code" placeholder="123456" bind:value={code} class="w-32" />
          </div>
          <Button onclick={confirmEnroll}>Подтвердить</Button>
        </div>
      </div>
    {:else if enabled}
      <p class="mt-2 text-sm text-emerald-400">2FA включена</p>
      <div class="mt-3 flex items-end gap-3">
        <div>
          <Label for="disable-code">Код для отключения</Label>
          <Input id="disable-code" placeholder="123456" bind:value={disableCode} class="w-32" />
        </div>
        <Button variant="destructive" onclick={turnOff}>Отключить</Button>
      </div>
    {:else}
      <p class="mt-2 text-sm text-muted-foreground">2FA не включена.</p>
      <Button class="mt-3" onclick={startEnroll}>Подключить 2FA</Button>
    {/if}

    {#if twofaMsg && !enroll && !enabled}
      <p class="mt-2 text-sm {twofaOk ? 'text-emerald-400' : 'text-red-400'}">{twofaMsg}</p>
    {/if}
  </section>
</div>
