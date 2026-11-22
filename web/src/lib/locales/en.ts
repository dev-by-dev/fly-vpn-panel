import type ru from './ru';

const en: Record<keyof typeof ru, string> = {
  'nav.home': 'Dashboard',
  'nav.users': 'Users',
  'nav.users.list': 'Users',
  'nav.users.devices': 'HWID / devices',
  'nav.inbounds': 'Inbounds',
  'nav.inbounds.list': 'Inbounds',
  'nav.inbounds.create': 'Add inbound',
  'nav.inbounds.metrics': 'Metrics',
  'nav.templates': 'Templates',
  'nav.settings': 'Settings',
  'nav.config': 'Config',
  'nav.logout': 'Log out',
  'common.save': 'Save',
  'common.cancel': 'Cancel',
  'common.delete': 'Delete',
  'common.close': 'Close',
  'common.copy': 'Copy',
  'common.refresh': 'Refresh',
  'common.error': 'Error',
  'common.network': 'Network unavailable',
};

export default en;