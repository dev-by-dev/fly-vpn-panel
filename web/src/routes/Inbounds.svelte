<script lang="ts">
  import { onMount } from 'svelte';
  import { apiFetch } from '$lib/api';
  import { Button } from '$lib/components/ui/button';
  import InboundForm from '$lib/components/InboundForm.svelte';

  type InboundRow = { id: number; tag: string; enabled: boolean; config: Record<string, any> };

  let inbounds = $state<InboundRow[]>([]);
  let error = $state('');
  let editing = $state<InboundRow | 'create' | null>(null);

  const applyWarn = 'Сохранено в БД, но применить не удалось — xray работает со старым конфигом, повторите позже.';

  function cfgPort(inb: InboundRow): string {
    return String(inb.config.port ?? '?');
  }

  function cfgPath(inb: InboundRow): string {
    return inb.config.streamSettings?.xhttpSettings?.path ?? '?';
  }

  async function load() {
    try {
      const res = await apiFetch('/api/inbounds');
      if (!res.ok) {
        error = `Не удалось загрузить (${res.status})`;
        return;
      }
      inbounds = (await res.json()) as InboundRow[];
      error = '';
    } catch {
      error = 'Сеть недоступна';
    }
  }

  onMount(load);

  async function closeAndReload() {
    editing = null;
    await load();
  }

  async function toggle(inb: InboundRow, enabled: boolean) {
    const res = await apiFetch(`/api/inbounds/${inb.id}`, {
      method: 'PATCH',
      body: JSON.stringify({ enabled }),
    });
    if (res.status === 502) alert(applyWarn);
    await load();
  }

  async function regenKeys(inb: InboundRow) {
    if (!confirm(`Перегенерировать Reality-ключи для ${inb.tag}? Клиентам нужен новый publicKey.`)) return;
    const res = await apiFetch(`/api/inbounds/${inb.id}/keys`, { method: 'POST' });
    if (res.status === 502) {
      alert(applyWarn);
    } else if (res.ok) {
      const data = (await res.json()) as { public_key: string };
      alert(`Новый publicKey: ${data.public_key}`);
    }
    await load();
  }

  async function remove(inb: InboundRow) {
    if (!confirm(`Удалить ${inb.tag}?`)) return;
    const res = await apiFetch(`/api/inbounds/${inb.id}`, { method: 'DELETE' });
    if (res.status === 502) alert(applyWarn);
    await load();
  }
</script>

<div class="flex items-center justify-between">
  <h1 class="text-2xl font-semibold">Inbounds</h1>
  <Button onclick={() => (editing = 'create')}>Добавить</Button>
</div>

<p class="mt-2 text-sm text-yellow-400">Внимание: применение изменений = рестарт xray ~1–2 с</p>

{#if error}
  <p class="mt-2 text-sm text-red-400">{error}</p>
{/if}

<div class="mt-4 overflow-x-auto rounded-xl border">
  <table class="w-full text-sm">
    <thead class="bg-muted/50 text-left text-muted-foreground">
      <tr>
        <th class="px-3 py-2 font-medium">Tag</th>
        <th class="px-3 py-2 font-medium">Порт</th>
        <th class="px-3 py-2 font-medium">Path</th>
        <th class="px-3 py-2 font-medium">Включён</th>
        <th class="px-3 py-2 font-medium">Действия</th>
      </tr>
    </thead>
    <tbody>
      {#each inbounds as inb (inb.id)}
        <tr class="border-t">
          <td class="px-3 py-2 font-medium">{inb.tag}</td>
          <td class="px-3 py-2">{cfgPort(inb)}</td>
          <td class="px-3 py-2">{cfgPath(inb)}</td>
          <td class="px-3 py-2">
            <input
              type="checkbox"
              checked={inb.enabled}
              onchange={(e) => toggle(inb, e.currentTarget.checked)}
              class="size-4 accent-primary"
            />
          </td>
          <td class="px-3 py-2">
            <div class="flex flex-wrap gap-1">
              <Button size="xs" variant="outline" onclick={() => (editing = inb)}>Изменить</Button>
              <Button size="xs" variant="outline" onclick={() => regenKeys(inb)}>Ключи</Button>
              <Button size="xs" variant="destructive" onclick={() => remove(inb)}>Удалить</Button>
            </div>
          </td>
        </tr>
      {/each}
    </tbody>
  </table>
</div>

{#if editing}
  <InboundForm
    inbound={editing === 'create' ? null : editing}
    onsaved={closeAndReload}
    onclose={() => (editing = null)}
  />
{/if}
