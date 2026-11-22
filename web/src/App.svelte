<script lang="ts">
  import { onMount } from 'svelte';
  import Router, { router, replace } from 'svelte-spa-router';
  import active from 'svelte-spa-router/active';
  import { authState, checkSession, logout } from '$lib/auth.svelte';
  import { usersIntent } from '$lib/usersIntent.svelte';
  import { inboundsIntent } from '$lib/inboundsIntent.svelte';
  import { i18n, langs, setLang, t } from '$lib/i18n.svelte';
  import { Button } from '$lib/components/ui/button';
  import Login from './routes/Login.svelte';
  import Dashboard from './routes/Dashboard.svelte';
  import Users from './routes/Users.svelte';
  import Inbounds from './routes/Inbounds.svelte';
  import Templates from './routes/Templates.svelte';
  import Monitoring from './routes/Monitoring.svelte';
  import Settings from './routes/Settings.svelte';
  import Config from './routes/Config.svelte';

  let usersMenu = $state(false);

  const routes = {
    '/': Dashboard,
    '/users': Users,
    '/inbounds': Inbounds,
    '/routing': Config,
    '/templates': Templates,
    '/monitoring': Monitoring,
    '/settings': Settings,
    '/config': Config,
    '/login': Login,
  };

  const nav = [
    { href: '#/', path: '/', labelKey: 'nav.home' },
    { href: '#/users', path: '/users', labelKey: 'nav.users' },
    { href: '#/inbounds', path: '/inbounds', labelKey: 'nav.inbounds' },
    { href: '#/templates', path: '/templates', labelKey: 'nav.templates' },
    { href: '#/monitoring', path: '/monitoring', labelKey: 'nav.monitoring' },
    { href: '#/settings', path: '/settings', labelKey: 'nav.settings' },
    { href: '#/config', path: '/config', labelKey: 'nav.config' },
  ] as const;

  // Выпадающее меню вкладки «Inbounds».
  let inbMenu = $state(false);

  onMount(checkSession);

  // Залогиненный на /login видел карточку входа внутри рабочего лэйаута.
  $effect(() => {
    if (authState.loggedIn && router.location === '/login') void replace('/');
  });
</script>

{#if !authState.loggedIn}
  <div class="flex min-h-screen items-center justify-center">
    <Login />
  </div>
{:else}
  <div class="flex min-h-screen flex-col">
    <header class="border-b bg-card">
      <div class="flex items-center gap-4 px-4">
        <div class="shrink-0 py-4 text-lg font-semibold">flypanel</div>
        <nav class="flex flex-1 items-center gap-1">
          {#each nav as item (item.href)}
            {#if item.path === '/users'}
            <div
              class="relative shrink-0"
              role="none"
              onmouseenter={() => (usersMenu = true)}
              onmouseleave={() => (usersMenu = false)}
            >
              <button
                type="button"
                aria-haspopup="menu"
                aria-expanded={usersMenu}
                class="-mb-px border-b-2 px-3 py-4 text-sm hover:text-foreground {router.location?.startsWith(
                  '/users',
                )
                  ? 'border-primary font-medium text-foreground'
                  : 'border-transparent text-muted-foreground'}"
                onclick={() => (usersMenu = !usersMenu)}
              >
                {t(item.labelKey)} ▾
              </button>
              {#if usersMenu}
                <div class="absolute left-0 top-full z-40 w-56 rounded-xl border bg-card p-1 shadow-lg">
                  <a
                    href="#/users"
                    class="block rounded px-2 py-1.5 text-sm hover:bg-muted"
                    onclick={() => {
                      usersIntent.view = 'list';
                      usersMenu = false;
                    }}>{t('nav.users.list')}</a
                  >
                  <button
                    type="button"
                    class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted"
                    onclick={() => {
                      usersIntent.view = 'devices';
                      usersMenu = false;
                      window.location.hash = '#/users';
                    }}>{t('nav.users.devices')}</button
                  >
                </div>
              {/if}
            </div>
          {:else if item.path === '/inbounds'}
              <div
                class="relative shrink-0"
                role="none"
                onmouseenter={() => (inbMenu = true)}
                onmouseleave={() => (inbMenu = false)}
              >
                <button
                  type="button"
                  aria-haspopup="menu"
                  aria-expanded={inbMenu}
                  class="-mb-px border-b-2 px-3 py-4 text-sm hover:text-foreground {router.location?.startsWith(
                    '/inbounds',
                  )
                    ? 'border-primary font-medium text-foreground'
                    : 'border-transparent text-muted-foreground'}"
                  onclick={() => (inbMenu = !inbMenu)}
                >
                  {t(item.labelKey)} ▾
                </button>
                {#if inbMenu}
                  <div class="absolute left-0 top-full z-40 w-56 rounded-xl border bg-card p-1 shadow-lg">
                    <a
                      href="#/inbounds"
                      class="block rounded px-2 py-1.5 text-sm hover:bg-muted"
                      onclick={() => (inbMenu = false)}>{t('nav.inbounds.list')}</a
                    >
                    <button
                      type="button"
                      class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted"
                      onclick={() => {
                        inboundsIntent.action = 'create';
                        inbMenu = false;
                        window.location.hash = '#/inbounds';
                      }}>{t('nav.inbounds.create')}</button
                    >
                    <button
                      type="button"
                      class="block w-full rounded px-2 py-1.5 text-left text-sm hover:bg-muted"
                      onclick={() => {
                        inboundsIntent.action = 'metrics';
                        inbMenu = false;
                        window.location.hash = '#/inbounds';
                      }}>{t('nav.inbounds.metrics')}</button
                    >
                  </div>
                {/if}
              </div>
            {:else}
              <a
                href={item.href}
                use:active={{
                  className: 'border-primary font-medium text-foreground',
                  inactiveClassName: 'border-transparent text-muted-foreground',
                }}
                class="-mb-px shrink-0 border-b-2 px-3 py-4 text-sm hover:text-foreground"
              >
                {t(item.labelKey)}
              </a>
            {/if}
          {/each}
        </nav>
        <div class="flex shrink-0 items-center gap-1 pr-2">
          {#each langs as l (l)}
            <button
              type="button"
              class="rounded px-1.5 py-1 text-xs uppercase {i18n.lang === l
                ? 'bg-muted font-semibold text-foreground'
                : 'text-muted-foreground hover:text-foreground'}"
              onclick={() => setLang(l)}>{l.toUpperCase()}</button
            >
          {/each}
        </div>
        <Button variant="outline" size="sm" class="shrink-0" onclick={() => logout()}
          >{t('nav.logout')}</Button
        >
      </div>
    </header>
    <main class="min-w-0 flex-1 p-6">
      <Router {routes} />
    </main>
  </div>
{/if}