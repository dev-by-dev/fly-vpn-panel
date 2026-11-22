<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import { t } from '$lib/i18n.svelte';

  const formats = [
    { id: 'json', label: 'json' },
    { id: 'mihomo', label: 'mihomo' },
    { id: 'clash', label: 'clash' },
  ] as const;
  type Format = (typeof formats)[number]['id'];

  // Placeholder sets mirror internal/sub renderers: json.go подставляет
  // только набор ниже; yaml.go exposes только {{PROXIES}}/{{PROXY_NAMES}} —
  // per-proxy поля (UUID/ADDRESS/…) живут внутри сгенерированной подстановки.
  const hints = (f: Format): string =>
    ({
    json: '{{UUID}} {{ADDRESS}} {{PORT}} {{NETWORK}} {{PBK}} {{SID}} {{SNI}} {{FP}} {{PATH}} {{MODE}} {{TAG}}',
    mihomo: `{{PROXIES}} {{PROXY_NAMES}} ${t('templates.proxiesHint')}`,
    clash: `{{PROXIES}} {{PROXY_NAMES}} ${t('templates.proxiesHint')}`,
    })[f];

  let active = $state<Format>('json');
  let templates = $state<Record<string, string>>({});
  let msg = $state('');
  let ok = $state(false);
  let error = $state('');

  async function load(f: Format) {
    try {
      const res = await apiFetch(`/api/templates/${f}`);
      if (!res.ok) {
        error = t('templates.loadFail', { status: res.status });
        return;
      }
      templates[f] = await res.text();
      error = '';
    } catch {
      error = t('common.network');
    }
  }

  onMount(async () => {
    for (const f of formats) await load(f.id);
  });

  async function save() {
    msg = '';
    try {
      const res = await apiFetch(`/api/templates/${active}`, {
        method: 'PUT',
        body: templates[active] ?? '',
        headers: { 'Content-Type': 'text/plain' },
      });
      if (res.ok) {
        ok = true;
        msg = t('templates.saved');
      } else {
        const data = (await res.json().catch(() => ({}))) as { error?: string };
        ok = false;
        msg = data.error === 'broken_template' ? t('templates.broken') : t('config.errGeneric', { status: res.status, code: '' });
      }
    } catch {
      ok = false;
      msg = t('common.network');
    }
  }

  function switchTab(f: Format) {
    active = f;
    msg = '';
  }
</script>

<h1 class="text-2xl font-semibold">{t('templates.title')}</h1>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{/if}

<div class="mt-4 flex gap-1">
  {#each formats as f (f.id)}
    <Button size="xs" variant={active === f.id ? 'default' : 'outline'} onclick={() => switchTab(f.id)}
      >{f.label}</Button
    >
  {/each}
</div>

<p class="mt-3 text-xs text-muted-foreground">
  {t('templates.placeholders')} <code class="text-foreground">{hints(active)}</code>
</p>

<textarea
  bind:value={templates[active]}
  spellcheck="false"
  class="mt-2 h-[60vh] w-full rounded-xl border bg-muted/30 p-4 font-mono text-xs"
></textarea>

<div class="mt-3 flex items-center gap-3">
  <Button onclick={save}>{t('templates.save')}</Button>
  {#if msg}
    <span class="text-sm {ok ? 'text-emerald-400' : 'text-red-400'}">{msg}</span>
  {/if}
</div>
