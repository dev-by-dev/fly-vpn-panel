<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';

  let text = $state('');
  let checkMsg = $state('');
  let checkOk = $state(false);
  let applyMsg = $state('');
  let applyOk = $state(false);
  let error = $state('');

  onMount(async () => {
    try {
      const res = await apiFetch('/api/config/routing');
      if (!res.ok) {
        error = `Не удалось загрузить (${res.status})`;
        return;
      }
      const raw = await res.text();
      try {
        text = JSON.stringify(JSON.parse(raw), null, 2);
      } catch {
        text = raw; // уже не-JSON — покажем как есть
      }
    } catch {
      error = 'Сеть недоступна';
    }
  });

  function check() {
    try {
      JSON.parse(text);
      checkOk = true;
      checkMsg = 'JSON корректен';
    } catch (e) {
      checkOk = false;
      checkMsg = `Некорректный JSON: ${(e as Error).message}`;
    }
  }

  async function apply() {
    applyMsg = '';
    try {
      const res = await apiFetch('/api/config/routing', { method: 'PUT', body: text });
      if (res.ok) {
        applyOk = true;
        applyMsg = 'Применено';
        return;
      }
      applyOk = false;
      const data = (await res.json().catch(() => ({}))) as { error?: string };
      if (res.status === 422) {
        applyMsg =
          data.error === 'invalid_json'
            ? 'Некорректный JSON (422)'
            : 'Применение не удалось (422), конфиг откачен к прежнему значению';
      } else if (res.status === 502) {
        applyMsg = 'xray недоступен (502)';
      } else {
        applyMsg = `Ошибка (${res.status}${data.error ? `: ${data.error}` : ''})`;
      }
    } catch {
      applyOk = false;
      applyMsg = 'Сеть недоступна';
    }
  }
</script>

<h1 class="text-2xl font-semibold">Routing</h1>
<p class="mt-2 text-sm text-muted-foreground">
  ⚠ Применение сразу изменяет работающий xray; при ошибке применения значение откатывается к прежнему.
</p>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{:else}
  <textarea
    bind:value={text}
    spellcheck="false"
    class="mt-4 h-[60vh] w-full rounded-xl border bg-muted/30 p-4 font-mono text-xs"
  ></textarea>
  <div class="mt-3 flex items-center gap-3">
    <Button variant="outline" onclick={check}>Проверить</Button>
    <Button onclick={apply}>Применить</Button>
    {#if checkMsg}
      <span class="text-sm {checkOk ? 'text-emerald-400' : 'text-red-400'}">{checkMsg}</span>
    {:else if applyMsg}
      <span class="text-sm {applyOk ? 'text-emerald-400' : 'text-red-400'}">{applyMsg}</span>
    {/if}
  </div>
{/if}
