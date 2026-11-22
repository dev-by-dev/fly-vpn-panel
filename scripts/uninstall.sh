#!/usr/bin/env bash
# flypanel uninstaller: Ubuntu 24.04 / CentOS Stream 9-10, x86_64.
# Removes what install.sh created. Panel data is deleted.
set -euo pipefail

UNINSTALL_LOG=${UNINSTALL_LOG:-/var/log/flypanel-uninstall.log}
OS_RELEASE_FILE=${OS_RELEASE_FILE:-/etc/os-release}
SSHD_T_CMD=${SSHD_T_CMD:-sshd -T}
PANEL_DIR=/opt/flypanel
CADDY_DIR=${CADDY_DIR:-/etc/caddy}
ACCESS_PORT=3002
# Written by install.sh: only the packages the installer itself added.
PKG_FILE=${PKG_FILE:-$PANEL_DIR/installed-packages}

DRY_RUN=false
ASSUME_YES=false
SKIP_FIREWALL=false
KEEP_PACKAGES=false
FORCE=false
LANG_SEL=${LANG_SEL:-}
FAMILY=""
CADDY_OURS=false
PKGS=""

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
  [step_packages]="Пакеты"
  [step_firewall]="Фаервол"
  [step_selinux]="SELinux"
  [step_logs]="Логи"
  [ok]="готово"
  [skip]="пропущено"
  [dry]="[dry-run]"
  [nothing]="Удалять нечего: панель не установлена."
  [done]="Готово. Машина очищена."
  [keep_notes]="Оставлено намеренно: правило SSH-порта и всё, что было на машине до установки."
  [pkg_none]="Пакетов, поставленных установщиком, нет."
  [pkg_kept]="Пакеты оставлены (--keep-packages)."
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
  [step_packages]="Packages"
  [step_firewall]="Firewall"
  [step_selinux]="SELinux"
  [step_logs]="Logs"
  [ok]="done"
  [skip]="skipped"
  [dry]="[dry-run]"
  [nothing]="Nothing to remove: flypanel is not installed."
  [done]="Done. The server has been cleaned up."
  [keep_notes]="Kept on purpose: the SSH rule and everything that was on the machine before the install."
  [pkg_none]="No packages were installed by the installer."
  [pkg_kept]="Packages kept (--keep-packages)."
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
  [step_packages]="Pakete"
  [step_firewall]="Firewall"
  [step_selinux]="SELinux"
  [step_logs]="Logs"
  [ok]="fertig"
  [skip]="übersprungen"
  [dry]="[dry-run]"
  [nothing]="Nichts zu löschen: flypanel ist nicht installiert."
  [done]="Fertig. Der Server ist bereinigt."
  [keep_notes]="Absichtlich behalten: die SSH-Regel und alles, was vor der Installation da war."
  [pkg_none]="Der Installer hat keine Pakete hinzugefügt."
  [pkg_kept]="Pakete behalten (--keep-packages)."
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

# quiet: run without output. The --dry-run line still shows, otherwise the
# firewall and SELinux steps report "done" with nothing to see.
quiet() {
  if $DRY_RUN; then
    printf '   %s %s\n' "$(t dry)" "$*"
    return 0
  fi
  "$@" >/dev/null 2>&1 || true
}

remove_path() { [ -e "$1" ] || return 0; run rm -rf "$1"; }

# caddyfile_has_content: true when something other than a leading global
# block (and comments/blanks) is left in the file.
caddyfile_has_content() {
  awk '
    function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
    { line = trim($0) }
    line == "" || line ~ /^#/ { next }
    ing == 0 && line == "{" { ing = 1; next }
    ing == 1 { if (line == "}") ing = 0; next }
    { found = 1 }
    END { exit found ? 0 : 1 }
  ' "$1"
}

# caddyfile_strip PATH -> 0 file kept without our block, 1 our file removed,
# 2 no markers (foreign config, untouched).
caddyfile_strip() {
  local path=$1
  [ -f "$path" ] || return 2
  # Both markers exactly once: a begin without end would swallow the rest of
  # the file, foreign sites included.
  [ "$(grep -c '^# flypanel-managed begin[[:space:]]*$' "$path")" -eq 1 ] || return 2
  [ "$(grep -c '^# flypanel-managed end[[:space:]]*$' "$path")" -eq 1 ] || return 2
  local tmp
  tmp=$(mktemp)
  awk '
    /^# flypanel-managed begin[[:space:]]*$/ { skip = 1; next }
    /^# flypanel-managed end[[:space:]]*$/   { skip = 0; next }
    !skip { print }
  ' "$path" > "$tmp"
  if ! caddyfile_has_content "$tmp"; then
    rm -f "$tmp"
    remove_path "$path"
    return 1
  fi
  if $DRY_RUN; then
    printf '   %s rewrite %s\n' "$(t dry)" "$path"
    rm -f "$tmp"
    return 0
  fi
  cat "$tmp" > "$path"
  rm -f "$tmp"
  # Caddy runs as its own user: the file must stay world-readable.
  chmod 644 "$path" 2>/dev/null || true
  return 0
}

# caddy_unit_is_ours: our unit points at /usr/local/bin/caddy. An unreadable
# unit is still ours as far as a dry-run can tell (the real run is root).
caddy_unit_is_ours() {
  local unit=/etc/systemd/system/caddy.service
  [ -f "$unit" ] || return 1
  grep -q '/usr/local/bin/caddy' "$unit" 2>/dev/null && return 0
  [ -r "$unit" ] || $DRY_RUN
}

stop_units() {
  step step_units
  local u unit
  for u in flypanel caddy; do
    unit=/etc/systemd/system/$u.service
    if [ ! -f "$unit" ]; then continue; fi
    if [ "$u" = caddy ] && ! caddy_unit_is_ours; then
      # Foreign caddy: keep it, and keep its binary and user too.
      continue
    fi
    [ "$u" = caddy ] && CADDY_OURS=true
    if systemctl is-active --quiet "$u"; then run systemctl stop "$u"; fi
    if systemctl is-enabled --quiet "$u" >/dev/null 2>&1; then run systemctl disable "$u"; fi
    remove_path "$unit"
  done
  run systemctl daemon-reload
  ok
}

remove_caddy_config() {
  step step_caddy
  local file=$CADDY_DIR/Caddyfile rc=0 f
  if [ ! -f "$file" ]; then say skip; return 0; fi
  caddyfile_strip "$file" || rc=$?
  case $rc in
    0) say caddy_kept ;;
    2) say caddy_foreign ;;
    1)
      # Our backups go with our file, otherwise the directory never empties.
      for f in "$CADDY_DIR"/Caddyfile.bak-*; do
        if [ -e "$f" ]; then remove_path "$f"; fi
      done
      run rmdir -- "$CADDY_DIR" 2>/dev/null || true
      ;;
  esac
  ok
}

remove_files() {
  step step_files
  remove_path "$PANEL_DIR"
  if $CADDY_OURS; then
    remove_path /usr/local/bin/caddy
    remove_path /var/lib/caddy
  fi
  ok
}

remove_users() {
  step step_user
  local u removed=0
  for u in flypanel caddy; do
    if [ "$u" = caddy ] && ! $CADDY_OURS; then continue; fi
    if ! id -u "$u" >/dev/null 2>&1; then continue; fi
    if run userdel "$u"; then
      removed=1
    elif ! $FORCE; then
      sayf err_user_busy "$u"
    fi
  done
  if [ "$removed" -eq 1 ] || $DRY_RUN; then ok; else say skip; fi
}

# detect_ssh_port: the port sshd listens on; empty when unknown.
detect_ssh_port() {
  local port=""
  port=$($SSHD_T_CMD 2>/dev/null | awk '$1 == "port" { print $2; exit }')
  [ -n "$port" ] || port=$(ss -ltnH 2>/dev/null | awk '$4 ~ /:22$/ { print 22; exit }')
  printf '%s' "$port"
}

# Ports we opened. The SSH port is never here on purpose: removing it can
# lock the operator out of the machine.
firewall_ports() {
  local p ssh
  ssh=$(detect_ssh_port)
  for p in 80 443 8443; do
    [ -n "$ssh" ] && [ "$p" = "$ssh" ] && continue
    printf '%s\n' "$p"
  done
}

remove_firewall_rules() {
  step step_firewall
  if $SKIP_FIREWALL; then say skip; return 0; fi
  local p did=0
  if have ufw && ufw status 2>/dev/null | head -1 | grep -qi active; then
    for p in $(firewall_ports); do
      quiet ufw --force delete allow "$p/tcp"
    done
    did=1
  fi
  if have firewall-cmd && systemctl is-active --quiet firewalld; then
    for p in $(firewall_ports); do
      quiet firewall-cmd --permanent --remove-port="$p/tcp"
    done
    quiet firewall-cmd --reload
    did=1
  fi
  if [ "$did" -eq 1 ]; then ok; else say skip; fi
}

remove_selinux_ports() {
  step step_selinux
  if ! have getenforce || ! have semanage; then say skip; return 0; fi
  if [ "$(getenforce 2>/dev/null)" != Enforcing ]; then say skip; return 0; fi
  quiet semanage port -d -t http_port_t -p tcp 8443
  quiet semanage port -d -t http_port_t -p tcp "$ACCESS_PORT"
  ok
}

# read_packages: the packages install.sh added, space separated. An older
# install has no list, so fall back to the extras a base system lacks.
read_packages() {
  if [ -f "$PKG_FILE" ]; then
    tr '\n' ' ' < "$PKG_FILE" | sed 's/[[:space:]]*$//'
    return 0
  fi
  if have apt-get; then
    printf '%s' "ufw unzip"
  elif have dnf; then
    printf '%s' "firewalld policycoreutils-python-utils"
  fi
}

remove_packages() {
  step step_packages
  if $KEEP_PACKAGES; then say pkg_kept; return 0; fi
  if [ -z "$PKGS" ]; then say pkg_none; return 0; fi
  # firewalld would keep a half-removed firewall unit around.
  case " $PKGS " in *" firewalld "*) quiet systemctl disable --now firewalld ;; esac
  if have apt-get; then
    run env DEBIAN_FRONTEND=noninteractive apt-get purge -y -qq $PKGS
  elif have dnf; then
    run dnf remove -y -q $PKGS
  else
    say pkg_none
    return 0
  fi
  ok
}

parse_args() {
  while [ $# -gt 0 ]; do
    case $1 in
      --lang) LANG_SEL=${2:-}; shift 2 ;;
      --yes|-y) ASSUME_YES=true; shift ;;
      --dry-run) DRY_RUN=true; shift ;;
      --skip-firewall) SKIP_FIREWALL=true; shift ;;
      --keep-packages) KEEP_PACKAGES=true; shift ;;
      --force) FORCE=true; shift ;;
      -h|--help) usage; exit 0 ;;
      *) printf 'unknown argument: %s (see --help)\n' "$1" >&2; exit 2 ;;
    esac
  done
}

usage() {
  cat <<'EOF'
usage: uninstall.sh [--lang ru|en|de] [--yes] [--dry-run] [--skip-firewall] [--keep-packages] [--force]
EOF
}

pick_lang() {
  case ${LANG_SEL:-} in
    ru|en|de) return 0 ;;
  esac
  local guess=ru
  case ${LANG:-} in
    de*) guess=de ;;
    en*) guess=en ;;
  esac
  LANG_SEL=$guess
  if [ -t 0 ] && ! $ASSUME_YES; then
    printf '%s' "$(t lang_ask)"
    local answer
    read -r answer || answer=""
    case $answer in
      1) LANG_SEL=ru ;;
      2) LANG_SEL=en ;;
      3) LANG_SEL=de ;;
    esac
  fi
}

confirm_removal() {
  say warn_data
  if $ASSUME_YES || $DRY_RUN; then return 0; fi
  if [ ! -t 0 ]; then say aborted; exit 1; fi
  printf '%s' "$(t ask_delete)"
  local ans
  read -r ans || ans=""
  if ! word_ok "$ans"; then
    say wrong_word
    say aborted
    exit 1
  fi
}

PANEL_UNIT=${PANEL_UNIT:-/etc/systemd/system/flypanel.service}

# panel_installed: the firewall and SELinux rules are ours only when the panel
# is; a hand-installed Caddy elsewhere on the box must not lose its ports.
panel_installed() { [ -e "$PANEL_DIR" ] || [ -e "$PANEL_UNIT" ]; }

installed_something() {
  panel_installed && return 0
  caddy_unit_is_ours && return 0
  return 1
}

summary() {
  say done
  say keep_notes
}

main() {
  parse_args "$@"
  pick_lang
  # Root before the log is opened: an unprivileged run must not print errors.
  if [ "$(id -u)" -ne 0 ] && ! $DRY_RUN; then
    sayf need_root "$0"
    exit 1
  fi
  say welcome
  if ! installed_something; then
    say nothing
    exit 0
  fi
  confirm_removal
  if [ -t 1 ] && ! $DRY_RUN; then
    exec > >(tee -a "$UNINSTALL_LOG") 2>&1
  fi

  local panel=0
  if panel_installed; then panel=1; fi
  # Read before remove_files: the list lives inside $PANEL_DIR.
  PKGS=$(read_packages)

  stop_units
  remove_caddy_config
  remove_files
  remove_users
  if [ "$panel" -eq 1 ] || $DRY_RUN; then
    remove_firewall_rules
    remove_selinux_ports
  fi
  remove_packages

  step step_logs
  remove_path /var/log/flypanel-install.log
  ok
  summary
  run rm -f "$UNINSTALL_LOG"
}

[[ ${UNINSTALL_LIB_ONLY:-0} == 1 ]] || main "$@"