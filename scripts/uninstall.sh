#!/usr/bin/env bash
# flypanel uninstaller: Ubuntu 24.04 / CentOS Stream 9-10, x86_64.
# Removes what install.sh created. Panel data is deleted.
set -euo pipefail

UNINSTALL_LOG=${UNINSTALL_LOG:-/var/log/flypanel-uninstall.log}
OS_RELEASE_FILE=${OS_RELEASE_FILE:-/etc/os-release}
PANEL_DIR=/opt/flypanel
ACCESS_PORT=3002

DRY_RUN=false
ASSUME_YES=false
SKIP_FIREWALL=false
FORCE=false
LANG_SEL=${LANG_SEL:-}
FAMILY=""

declare -A MSG_RU=(
  [welcome]="Удаление панели flypanel"
  [lang_ask]="Выбери язык / Choose language / Sprache wählen [1] ru [2] en [3] de: "
  [need_root]="Нужен root: sudo bash %s"
  [unsupported_os]="Неизвестная ОС: %s. Продолжаю осторожно (--force убирает предупреждение)."
  [warn_data]="Внимание: будут удалены панель, xray, Caddy (если ставили мы) и ВСЕ данные: юзеры, токены, подписки. Откатить нельзя."
  [ask_delete]="Чтобы продолжить, введи слово delete: "
  [wrong_word]="Слово не совпало — ничего не удалено."
  [aborted]="Отменено."
  [step_units]="systemd"
  [step_caddy]="Caddy"
  [step_files]="Файлы"
  [step_user]="Пользователи"
  [step_firewall]="Фаервол"
  [step_selinux]="SELinux"
  [step_logs]="Логи"
  [ok]="готово"
  [skip]="пропущено"
  [dry]="[dry-run]"
  [nothing]="Удалять нечего: панель не установлена."
  [done]="Готово. Машина очищена."
  [keep_notes]="Оставлено намеренно: пакеты, фаервол включён, правило SSH-порта."
  [err_user_busy]="Не удалось удалить юзера %s: есть процессы (--force, чтобы продолжить)."
  [caddy_foreign]="В /etc/caddy/Caddyfile нет нашего блока — файл не трогаю."
  [caddy_kept]="Чужой сайт в Caddyfile остался, наш блок убран."
)

declare -A MSG_EN=(
  [welcome]="Removing flypanel"
  [lang_ask]="Выбери язык / Choose language / Sprache wählen [1] ru [2] en [3] de: "
  [need_root]="Root required: sudo bash %s"
  [unsupported_os]="Unknown OS: %s. Continuing carefully (--force silences the warning)."
  [warn_data]="Warning: the panel, xray, Caddy (if we installed it) and ALL data (users, tokens, subscriptions) will be deleted. This cannot be undone."
  [ask_delete]="Type the word delete to continue: "
  [wrong_word]="Word did not match — nothing was deleted."
  [aborted]="Aborted."
  [step_units]="systemd"
  [step_caddy]="Caddy"
  [step_files]="Files"
  [step_user]="Users"
  [step_firewall]="Firewall"
  [step_selinux]="SELinux"
  [step_logs]="Logs"
  [ok]="done"
  [skip]="skipped"
  [dry]="[dry-run]"
  [nothing]="Nothing to remove: flypanel is not installed."
  [done]="Done. The server has been cleaned up."
  [keep_notes]="Kept on purpose: packages, firewall stays enabled, SSH rule."
  [err_user_busy]="Could not remove user %s: running processes (--force to continue)."
  [caddy_foreign]="No flypanel block in /etc/caddy/Caddyfile — leaving the file alone."
  [caddy_kept]="Foreign site kept in the Caddyfile, our block removed."
)

declare -A MSG_DE=(
  [welcome]="flypanel entfernen"
  [lang_ask]="Выбери язык / Choose language / Sprache wählen [1] ru [2] en [3] de: "
  [need_root]="Root erforderlich: sudo bash %s"
  [unsupported_os]="Unbekanntes System: %s. Ich mache vorsichtig weiter (--force unterdrückt die Warnung)."
  [warn_data]="Achtung: Panel, xray, Caddy (falls von uns) und ALLE Daten (Benutzer, Tokens, Abos) werden gelöscht. Kein Zurück."
  [ask_delete]="Zum Fortfahren das Wort delete eingeben: "
  [wrong_word]="Wort stimmt nicht — nichts gelöscht."
  [aborted]="Abgebrochen."
  [step_units]="systemd"
  [step_caddy]="Caddy"
  [step_files]="Dateien"
  [step_user]="Benutzer"
  [step_firewall]="Firewall"
  [step_selinux]="SELinux"
  [step_logs]="Logs"
  [ok]="fertig"
  [skip]="übersprungen"
  [dry]="[dry-run]"
  [nothing]="Nichts zu löschen: flypanel ist nicht installiert."
  [done]="Fertig. Der Server ist bereinigt."
  [keep_notes]="Absichtlich behalten: Pakete, Firewall bleibt aktiv, SSH-Regel."
  [err_user_busy]="Benutzer %s konnte nicht gelöscht werden: laufende Prozesse (--force zum Fortsetzen)."
  [caddy_foreign]="Kein flypanel-Block in /etc/caddy/Caddyfile — Datei bleibt unverändert."
  [caddy_kept]="Fremde Site in der Caddyfile bleibt, unser Block ist entfernt."
)

t() {
  local k=$1 v=""
  case ${LANG_SEL:-ru} in
    ru) v=${MSG_RU[$k]-} ;;
    en) v=${MSG_EN[$k]-} ;;
    de) v=${MSG_DE[$k]-} ;;
  esac
  [ -n "$v" ] || v=${MSG_RU[$k]-}
  printf '%s' "$v"
}

say() { printf '%s\n' "$(t "$1")"; }
sayf() { local k=$1; shift; printf "$(t "$k")\n" "$@"; }
step() { printf '\n== %s\n' "$(t "$1")"; }
ok() { printf '   %s\n' "$(t ok)"; }

have() { command -v "$1" >/dev/null 2>&1; }

# run executes a command, or prints it in --dry-run mode.
run() {
  if $DRY_RUN; then
    printf '   %s %s\n' "$(t dry)" "$*"
    return 0
  fi
  "$@"
}

word_ok() { [ "${1:-}" = delete ]; }

remove_path() { [ -e "$1" ] || return 0; run rm -rf "$1"; }

main() { :; }

[[ ${UNINSTALL_LIB_ONLY:-0} == 1 ]] || main "$@"