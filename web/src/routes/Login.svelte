<script lang="ts">
  import { login, authState } from '$lib/auth.svelte';
  import { t } from '$lib/i18n.svelte';
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
      error = t('login.failed');
    }
  }
</script>

<Card class="w-80">
  <CardHeader>
    <CardTitle class="text-xl">flypanel</CardTitle>
    <CardDescription>{t('login.subtitle')}</CardDescription>
  </CardHeader>
  <CardContent>
    <form onsubmit={submit} class="flex flex-col gap-4">
      <div class="flex flex-col gap-2">
        <Label for="username">{t('login.username')}</Label>
        <Input id="username" bind:value={username} autocomplete="username" required />
      </div>
      <div class="flex flex-col gap-2">
        <Label for="password">{t('login.password')}</Label>
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
          <Label for="code">{t('login.code')}</Label>
          <Input id="code" bind:value={code} autocomplete="one-time-code" required />
        </div>
      {/if}
      {#if error}
        <p class="text-sm text-destructive">{error}</p>
      {/if}
      <Button type="submit" disabled={busy}>{busy ? t('login.busy') : t('login.submit')}</Button>
    </form>
  </CardContent>
</Card>
