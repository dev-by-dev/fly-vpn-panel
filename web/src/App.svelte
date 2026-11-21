<script lang="ts">
  import { onMount } from 'svelte';
  import Router from 'svelte-spa-router';
  import active from 'svelte-spa-router/active';
  import { authState, checkSession, logout } from '$lib/auth.svelte';
  import { Button } from '$lib/components/ui/button';
  import Login from './routes/Login.svelte';
  import Dashboard from './routes/Dashboard.svelte';
  import Users from './routes/Users.svelte';
  import Inbounds from './routes/Inbounds.svelte';
  import Routing from './routes/Routing.svelte';
  import Templates from './routes/Templates.svelte';
  import Settings from './routes/Settings.svelte';
  import Config from './routes/Config.svelte';

  const routes = {
    '/': Dashboard,
    '/users': Users,
    '/inbounds': Inbounds,
    '/routing': Routing,
    '/templates': Templates,
    '/settings': Settings,
    '/config': Config,
    '/login': Login,
  };

  const nav = [
    { href: '#/', path: '/', label: 'Дашборд' },
    { href: '#/users', path: '/users', label: 'Юзеры' },
    { href: '#/inbounds', path: '/inbounds', label: 'Inbounds' },
    { href: '#/routing', path: '/routing', label: 'Routing' },
    { href: '#/templates', path: '/templates', label: 'Шаблоны' },
    { href: '#/settings', path: '/settings', label: 'Настройки' },
    { href: '#/config', path: '/config', label: 'Конфиг' },
  ];

  onMount(checkSession);
</script>

{#if !authState.loggedIn}
  <div class="flex min-h-screen items-center justify-center">
    <Login />
  </div>
{:else}
  <div class="flex min-h-screen">
    <aside class="flex w-56 shrink-0 flex-col border-r bg-card">
      <div class="px-4 py-4 text-lg font-semibold">flypanel</div>
      <nav class="flex flex-col gap-1 px-2">
        {#each nav as item (item.href)}
          <a
            href={item.href}
            use:active={{
              className: 'bg-muted font-medium text-foreground',
              inactiveClassName: 'text-muted-foreground',
            }}
            class="rounded-md px-3 py-2 text-sm hover:bg-muted"
          >
            {item.label}
          </a>
        {/each}
      </nav>
      <div class="mt-auto p-2">
        <Button variant="outline" class="w-full" onclick={() => logout()}>Выйти</Button>
      </div>
    </aside>
    <main class="flex-1 p-6">
      <Router {routes} />
    </main>
  </div>
{/if}
