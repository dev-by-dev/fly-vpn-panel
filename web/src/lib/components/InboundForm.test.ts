import { test, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/svelte';
import InboundForm from './InboundForm.svelte';

afterEach(cleanup);

// Регресс: строка inbounds приходит из $state, т.е. это deep-proxy.
// structuredClone на прокси бросает DataCloneError и диалог не открывается.
const config = {
  port: 8443,
  protocol: 'vless',
  settings: { clients: [] },
  streamSettings: {
    network: 'xhttp',
    security: 'reality',
    xhttpSettings: { path: '/xhttp', mode: 'stream-up' },
    realitySettings: { serverNames: ['www.microsoft.com'], shortIds: ['e0cb7828'] },
  },
};
const proxyConfig = new Proxy(config, {});

test('inbound из $state-прокси рендерится без DataCloneError', () => {
  render(InboundForm, {
    props: {
      inbound: { id: 1, tag: 'vless-xhttp', enabled: true, config: proxyConfig as any },
      onsaved: () => {},
      onclose: () => {},
    },
  });

  expect(screen.getByText('Изменить inbound')).toBeTruthy();
});