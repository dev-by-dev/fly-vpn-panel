<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';

  let cfg = $state('');
  let notGenerated = $state(false);
  let error = $state('');
  let copied = $state(false);

  onMount(async () => {
    try {
      const res = await apiFetch('/api/config');
      if (res.status === 404) {
        notGenerated = true;
        return;
      }
      if (!res.ok) {
        error = `Не удалось загрузить (${res.status})`;
        return;
      }
      const text = await res.text();
      try {
        cfg = JSON.stringify(JSON.parse(text), null, 2);
      } catch {
        cfg = text;
      }
    } catch {
      error = 'Сеть недоступна';
    }
  });

  async function copy() {
    await navigator.clipboard?.writeText(cfg).catch(() => {});
    copied = true;
  }
</script>

<div class="flex items-center justify-between">
  <h1 class="text-2xl font-semibold">Конфиг</h1>
  {#if cfg}
    <Button variant="outline" onclick={copy}>{copied ? 'Скопировано' : 'Копировать'}</Button>
  {/if}
</div>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{:else if notGenerated}
  <p class="mt-2 text-sm text-muted-foreground">Конфиг ещё не сгенерирован.</p>
{:else}
  <pre
    class="mt-4 max-h-[70vh] overflow-auto rounded-xl border bg-muted/30 p-4 font-mono text-xs whitespace-pre-wrap"
  >{cfg}</pre>
{/if}
