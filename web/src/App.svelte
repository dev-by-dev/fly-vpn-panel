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
    { href: '#/', path: '/', label: 'Главная' },
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
  <div class="flex min-h-screen flex-col">
    <header class="border-b bg-card">
      <div class="flex items-center gap-4 px-4">
        <div class="shrink-0 py-4 text-lg font-semibold">flypanel</div>
        <nav class="flex flex-1 items-center gap-1 overflow-x-auto">
          {#each nav as item (item.href)}
            <a
              href={item.href}
              use:active={{
                className: 'border-primary font-medium text-foreground',
                inactiveClassName: 'border-transparent text-muted-foreground',
              }}
              class="-mb-px shrink-0 border-b-2 px-3 py-4 text-sm hover:text-foreground"
            >
              {item.label}
            </a>
          {/each}
        </nav>
        <Button variant="outline" size="sm" class="shrink-0" onclick={() => logout()}>Выйти</Button>
      </div>
    </header>
    <main class="min-w-0 flex-1 p-6">
      <Router {routes} />
    </main>
  </div>
{/if}