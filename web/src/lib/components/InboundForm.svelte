<script lang="ts">
  import { Button } from '$lib/components/ui/button';
  import { Input } from '$lib/components/ui/input';
  import { Label } from '$lib/components/ui/label';
  import { apiFetch } from '$lib/api';
  import { t } from '$lib/i18n.svelte';
  import { errText } from '$lib/errors';

  type InboundRow = { id: number; tag: string; enabled: boolean; config: Record<string, unknown> };

  type Props = { inbound: InboundRow | null; onsaved: () => void; onclose: () => void };
  let { inbound, onsaved, onclose }: Props = $props();

  const isEdit = inbound !== null;
  const cfg0: Record<string, any> = inbound?.config ?? {};
  const rs0 = cfg0.streamSettings?.realitySettings ?? {};
  const xh0 = cfg0.streamSettings?.xhttpSettings ?? {};

  let tag = $state(inbound?.tag ?? '');
  // Дефолты по спеке: 8443 и stream-up (443/auto ломали свежий inbound).
  let port = $state<number>(cfg0.port ?? 8443);
  let path = $state<string>(xh0.path ?? '/xhttp');
  let mode = $state<string>(xh0.mode ?? 'stream-up');
  let dest = $state<string>(rs0.dest ?? 'www.microsoft.com:443');
  let serverNames = $state<string>((rs0.serverNames ?? []).join(','));
  let shortIds = $state<string>((rs0.shortIds ?? []).join(','));
  let fp = $state<string>(rs0.fingerprint ?? 'chrome');
  let raw = $state('');
  let rawDirty = $state(false);
  let error = $state('');
  let saving = $state(false);

  // структурные поля пишутся в JSON; ручное редактирование textarea имеет приоритет
  function build(): Record<string, any> {
    // $state.snapshot: cfg0 may be a Svelte 5 proxy (row from $state array);
    // structuredClone throws DataCloneError on proxies.
    const cfg = $state.snapshot(cfg0) as Record<string, any>;
    cfg.port = port;
    const ss = (cfg.streamSettings ??= {});
    ss.network = 'xhttp';
    const xh = (ss.xhttpSettings ??= {});
    xh.path = path;
    xh.mode = mode;
    const rs = (ss.realitySettings ??= {});
    rs.dest = dest;
    rs.serverNames = serverNames.split(',').map((s) => s.trim()).filter(Boolean);
    rs.shortIds = shortIds.split(',').map((s) => s.trim()).filter(Boolean);
    rs.fingerprint = fp;
    if (cfg.protocol == null) cfg.protocol = 'vless';
    if (cfg.settings == null) cfg.settings = { clients: [] };
    return cfg;
  }

  function pretty(cfg: Record<string, unknown>): string {
    return JSON.stringify(cfg, null, 2);
  }

  async function submit(e: SubmitEvent) {
    e.preventDefault();
    error = '';
    if (tag.trim() === '') {
      error = t('form.errTag');
      return;
    }
    let cfg: Record<string, unknown>;
    try {
      cfg = rawDirty ? (JSON.parse(raw) as Record<string, unknown>) : build();
    } catch {
      error = t('form.errJson');
      return;
    }
    saving = true;
    try {
      const res = await apiFetch(isEdit ? `/api/inbounds/${inbound!.id}` : '/api/inbounds', {
        method: isEdit ? 'PATCH' : 'POST',
        body: JSON.stringify(isEdit ? { tag: tag.trim(), config: cfg } : { tag: tag.trim(), enabled: true, config: cfg }),
      });
      if (res.status === 502) {
        alert(t('inbounds.applyWarn'));
        onsaved();
        return;
      }
      if (!res.ok) {
        const data = (await res.json().catch(() => ({}))) as { error?: string };
        error = errText(data.error ?? '', t('dialog.errGeneric', { code: data.error ?? res.status }));
        return;
      }
      if (!isEdit) {
        // у нового inbound пустые Reality-ключи: генерируем сразу, иначе xray не применит конфиг
        const created = (await res.json()) as { id: number };
        const keys = await apiFetch(`/api/inbounds/${created.id}/keys`, { method: 'POST' });
        if (keys.status === 502) alert(t('inbounds.applyWarn'));
      }
      onsaved();
    } catch {
      error = t('common.network');
    } finally {
      saving = false;
    }
  }
</script>

<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" role="dialog" aria-modal="true">
  <div class="max-h-[90vh] w-full max-w-lg overflow-y-auto rounded-xl border bg-card p-5 shadow-lg">
    <h2 class="text-lg font-semibold">{isEdit ? t('form.editTitle') : t('form.newTitle')}</h2>
    <p class="mt-1 text-xs text-yellow-400">{t('form.restartNote')}</p>
    <form class="mt-4 grid gap-4" onsubmit={submit}>
      <div class="grid gap-1.5">
        <Label for="i-tag">Tag</Label>
        <Input id="i-tag" bind:value={tag} placeholder="in-443" />
      </div>
      <div class="grid gap-1.5">
        <Label for="i-port">{t('form.port')}</Label>
        <Input id="i-port" type="number" min="1" max="65535" bind:value={port} />
      </div>
      <div class="grid gap-1.5">
        <Label for="i-path">Path</Label>
        <Input id="i-path" bind:value={path} placeholder="/xhttp" />
      </div>
      <div class="grid gap-1.5">
        <Label for="i-mode">{t('form.mode')}</Label>
        <select
          id="i-mode"
          bind:value={mode}
          class="border-input bg-input/30 h-8 w-full rounded-lg border px-2.5 text-sm outline-none"
        >
          <option value="auto">auto</option>
          <option value="packet-up">packet-up</option>
          <option value="stream-up">stream-up</option>
          <option value="stream-one">stream-one</option>
        </select>
      </div>
      <div class="grid gap-1.5">
        <Label for="i-dest">{t('form.dest')}</Label>
        <Input id="i-dest" bind:value={dest} placeholder="www.microsoft.com:443" />
      </div>
      <div class="grid gap-1.5">
        <Label for="i-sni">{t('form.sni')}</Label>
        <Input id="i-sni" bind:value={serverNames} placeholder="www.microsoft.com" />
      </div>
      <div class="grid gap-1.5">
        <Label for="i-sids">{t('form.sids')}</Label>
        <Input id="i-sids" bind:value={shortIds} placeholder="" />
      </div>
      <div class="grid gap-1.5">
        <Label for="i-fp">{t('form.fp')}</Label>
        <select
          id="i-fp"
          bind:value={fp}
          class="border-input bg-input/30 h-8 w-full rounded-lg border px-2.5 text-sm outline-none"
        >
          {#each ['chrome', 'firefox', 'safari', 'ios', 'android', 'edge', 'randomized'] as f (f)}
            <option value={f}>{f}</option>
          {/each}
        </select>
      </div>
      <div class="grid gap-1.5">
        <Label for="i-raw">{t('form.raw')}</Label>
        <textarea
          id="i-raw"
          rows="10"
          class="border-input bg-input/30 w-full rounded-lg border p-2.5 font-mono text-xs outline-none"
          value={rawDirty ? raw : pretty(build())}
          oninput={(e) => {
            raw = e.currentTarget.value;
            rawDirty = true;
          }}
        ></textarea>
      </div>
      {#if error}
        <p class="text-sm text-red-400">{error}</p>
      {/if}
      <div class="mt-1 flex justify-end gap-2">
        <Button type="button" variant="outline" onclick={onclose}>{t('form.cancel')}</Button>
        <Button type="submit" disabled={saving}>{saving ? t('form.saving') : t('form.save')}</Button>
      </div>
    </form>
  </div>
</div>
