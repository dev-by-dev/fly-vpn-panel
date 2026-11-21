<script lang="ts">
  import { login, authState } from '$lib/auth.svelte';
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import {
    Card,
    CardContent,
    CardDescription,
    CardHeader,
    CardTitle,
  } from '$lib/components/ui/card';

  let username = $state('');
  let password = $state('');
  let code = $state('');
  let error = $state('');
  let busy = $state(false);

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    busy = true;
    error = '';
    const ok = await login(username, password, authState.need2fa ? code : undefined);
    busy = false;
    if (!ok && !authState.need2fa) {
      error = 'Неверный логин, пароль или код';
    }
  }
</script>

<Card class="w-80">
  <CardHeader>
    <CardTitle class="text-xl">flypanel</CardTitle>
    <CardDescription>Вход в панель управления</CardDescription>
  </CardHeader>
  <CardContent>
    <form onsubmit={submit} class="flex flex-col gap-4">
      <div class="flex flex-col gap-2">
        <Label for="username">Логин</Label>
        <Input id="username" bind:value={username} autocomplete="username" required />
      </div>
      <div class="flex flex-col gap-2">
        <Label for="password">Пароль</Label>
        <Input
          id="password"
          type="password"
          bind:value={password}
          autocomplete="current-password"
          required
        />
      </div>
      {#if authState.need2fa}
        <div class="flex flex-col gap-2">
          <Label for="code">Код 2FA (TOTP или резервный)</Label>
          <Input id="code" bind:value={code} autocomplete="one-time-code" required />
        </div>
      {/if}
      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}
      <Button type="submit" disabled={busy}>{busy ? 'Вход…' : 'Войти'}</Button>
    </form>
  </CardContent>
</Card>
