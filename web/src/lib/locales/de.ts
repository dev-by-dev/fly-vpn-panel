import type ru from './ru';

const de: Record<keyof typeof ru, string> = {
  'nav.home': 'Übersicht',
  'nav.users': 'Benutzer',
  'nav.users.list': 'Benutzer',
  'nav.users.devices': 'HWID / Geräte',
  'nav.inbounds': 'Inbounds',
  'nav.inbounds.list': 'Inbounds',
  'nav.inbounds.create': 'Inbound hinzufügen',
  'nav.inbounds.metrics': 'Metriken',
  'nav.templates': 'Vorlagen',
  'nav.settings': 'Einstellungen',
  'nav.config': 'Konfiguration',
  'nav.logout': 'Abmelden',
  'common.save': 'Speichern',
  'common.cancel': 'Abbrechen',
  'common.delete': 'Löschen',
  'common.close': 'Schließen',
  'common.copy': 'Kopieren',
  'common.refresh': 'Aktualisieren',
  'common.error': 'Fehler',
  'common.network': 'Netzwerk nicht erreichbar',
};

export default de;