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

  onMount(async () => {
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
</script>

<h1 class="text-2xl font-semibold">Настройки</h1>

<div class="mt-4 grid max-w-2xl grid-cols-1 gap-6">
  <!-- Password -->
  <section class="rounded-xl border border-sky-500/20 bg-sky-500/5 p-4">
    <h2 class="font-medium">Смена пароля</h2>
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
    <h2 class="font-medium">Интервал поллинга</h2>
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
    <h2 class="font-medium">Двухфакторная аутентификация</h2>

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
