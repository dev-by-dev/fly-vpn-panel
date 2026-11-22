<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { t } from '$lib/i18n.svelte';

  let cfg = $state('');
  let notGenerated = $state(false);
  let error = $state('');
  let copied = $state(false);

  // Routing-редактор: живёт отдельно от генерального конфига, ошибки независимы.
  let rText = $state('');
  let rCheckMsg = $state('');
  let rCheckOk = $state(false);
  let rApplyMsg = $state('');
  let rApplyOk = $state(false);
  let rError = $state('');

  onMount(async () => {
    void loadRouting();
    await loadConfig();
  });

  async function loadConfig() {
    error = '';
    notGenerated = false;
    try {
      const res = await apiFetch('/api/config');
      if (res.status === 404) {
        notGenerated = true;
        return;
      }
      if (!res.ok) {
        error = t('config.loadFail', { status: res.status });
        return;
      }
      const text = await res.text();
      try {
        cfg = JSON.stringify(JSON.parse(text), null, 2);
      } catch {
        cfg = text;
      }
    } catch {
      error = t('common.network');
    }
  }

  async function loadRouting() {
    try {
      const res = await apiFetch('/api/config/routing');
      if (!res.ok) {
        rError = t('config.routingLoadFail', { status: res.status });
        return;
      }
      const raw = await res.text();
      try {
        rText = JSON.stringify(JSON.parse(raw), null, 2);
      } catch {
        rText = raw; // уже не-JSON — покажем как есть
      }
    } catch {
      rError = t('common.network');
    }
  }

  function rCheck() {
    rApplyMsg = '';
    try {
      JSON.parse(rText);
      rCheckOk = true;
      rCheckMsg = t('config.jsonOk');
    } catch (e) {
      rCheckOk = false;
      rCheckMsg = t('config.jsonBad', { msg: (e as Error).message });
    }
  }

  async function rApply() {
    rCheckMsg = '';
    rApplyMsg = '';
    try {
      const res = await apiFetch('/api/config/routing', { method: 'PUT', body: rText });
      if (res.ok) {
        rApplyOk = true;
        rApplyMsg = t('config.applied');
        await loadConfig();
        return;
      }
      rApplyOk = false;
      const data = (await res.json().catch(() => ({}))) as { error?: string };
      if (res.status === 422) {
        rApplyMsg =
          data.error === 'invalid_json'
            ? t('config.invalidJson422')
            : t('config.applyFailed422');
      } else if (res.status === 502) {
        rApplyMsg = t('config.xrayDown502');
      } else {
        rApplyMsg = t('config.errGeneric', { status: res.status, code: data.error ? `: ${data.error}` : '' });
      }
    } catch {
      rApplyOk = false;
      rApplyMsg = t('common.network');
    }
  }

  async function copy() {
    await navigator.clipboard?.writeText(cfg).catch(() => {});
    copied = true;
  }
</script>

<div class="flex items-center justify-between">
  <h1 class="text-2xl font-semibold">{t('config.title')}</h1>
  {#if cfg}
    <Button variant="outline" onclick={copy}>{copied ? t('config.copied') : t('config.copy')}</Button>
  {/if}
</div>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{:else if notGenerated}
  <p class="mt-2 text-sm text-muted-foreground">{t('config.notGenerated')}</p>
{:else}
  <pre
    class="mt-4 max-h-[70vh] overflow-auto rounded-xl border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap"
  >{cfg}</pre>
{/if}

<h2 class="mt-8 text-lg font-semibold">Routing (advanced JSON)</h2>
<p class="mt-2 text-sm text-muted-foreground">
  {t('config.warn')}
</p>

{#if rError}
  <p class="mt-2 text-sm text-red-400">{rError}</p>
{:else}
  <textarea
    bind:value={rText}
    aria-label="Routing JSON"
    spellcheck="false"
    class="mt-4 h-[40vh] w-full rounded-xl border bg-muted/30 p-4 font-mono text-xs"
  ></textarea>
  <div class="mt-3 flex items-center gap-3">
    <Button variant="outline" onclick={rCheck}>{t('config.check')}</Button>
    <Button onclick={rApply}>{t('config.apply')}</Button>
    {#if rCheckMsg}
      <span class="text-sm {rCheckOk ? 'text-emerald-400' : 'text-red-400'}">{rCheckMsg}</span>
    {:else if rApplyMsg}
      <span class="text-sm {rApplyOk ? 'text-emerald-400' : 'text-red-400'}">{rApplyMsg}</span>
    {/if}
  </div>
{/if}
