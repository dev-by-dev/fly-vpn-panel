#!/usr/bin/env bash
# flypanel installer: Ubuntu 24.04 / CentOS Stream 9-10, x86_64.
# Idempotent: re-runs update the binary and the managed Caddy block only.
set -euo pipefail

INSTALL_LOG=${INSTALL_LOG:-/var/log/flypanel-install.log}
OS_RELEASE_FILE=${OS_RELEASE_FILE:-/etc/os-release}
SSHD_T_CMD=${SSHD_T_CMD:-sshd -T}
XRAY_VERSION=26.11.9
CADDY_VERSION=2.11.9
REPO=dev-by-dev/fly-vpn-panel
PANEL_DIR=/opt/flypanel
ACCESS_PORT=3002

DRY_RUN=false
FORCE=false
ASSUME_YES=false
SKIP_DNS=false
SKIP_FIREWALL=false
SKIP_UPDATE=false
HTTPS_PENDING=0
LANG_SEL=${LANG_SEL:-}
DOMAIN=${DOMAIN:-}
EMAIL=${EMAIL:-}
ADMIN_NAME=${ADMIN_NAME:-admin}
IP=${IP:-}
FAMILY=""
SSH_PORT=""
FIREWALL_FAILED=false
ADMIN_PASSWORD=""
ADMIN_CREATED=0
ADMIN_FAILED=false
PKG_NEW=""

# Ruling: associative arrays instead of numeric indexes — same single file,
# no off-by-one between the three dictionaries.
declare -A MSG_RU=(
  [welcome]="Установка панели flypanel"
  [lang_ask]="Выбери язык / Choose language / Sprache wählen [1] ru [2] en [3] de: "
  [need_root]="Нужен root: sudo bash %s"
  [unsupported_os]="Поддерживаются Ubuntu 24.04 и CentOS Stream 9/10 (x86_64). Нашлось: %s. Для ручной установки: docs/deploy-checklist.md"
  [no_systemd]="systemd не найден — это не VPS и не сервер."
  [container]="Похоже на контейнер: systemd/journald работать не будут."
  [disk_low]="Мало места на /: нужно 2 ГБ, свободно %s МБ."
  [port_busy]="Порт %s занят чужим процессом: %s"
  [ip_fail]="Не удалось определить публичный IP. Укажи вручную: --ip A.B.C.D"
  [ip_nat]="Внимание: IP %s может быть внутренним (NAT). Входящие подключения к VPN потребуют проброса портов."
  [dns_mismatch]="Домен %s не указывает на %s. Сначала A-запись в DNS, потом установка (или --skip-dns)."
  [dns_ok]="DNS: %s -> %s"
  [q_domain]="Домен панели (A-запись уже должна указывать на этот сервер): "
  [q_email]="E-mail для Let's Encrypt (можно пусто): "
  [q_admin]="Логин администратора [admin]: "
  [already_running]="Установщик уже запущен в другом терминале. Дождись окончания."
  [q_firewall]="Настроить фаервол (SSH-порт %s, 80, 443, 8443)? [Y/n]: "
  [confirm]="Установить flypanel на %s для домена %s? [Y/n]: "
  [aborted]="Отменено."
  [step_update]="Обновление системы"
  [step_packages]="Пакеты"
  [step_user]="Пользователь и каталоги"
  [step_panel]="Бинарь панели"
  [step_xray]="xray-core"
  [step_env]="Файл окружения"
  [step_unit]="systemd-юнит"
  [step_firewall]="Фаервол"
  [step_selinux]="SELinux"
  [step_caddy]="Caddy и HTTPS"
  [step_https]="Проверка HTTPS"
  [step_admin]="Администратор"
  [ok]="готово"
  [skip]="пропущено"
  [dry]="[dry-run]"
  [dry_preview]="Продолжаю в режиме просмотра: ничего не меняется."
  [admin_exists]="Админ уже создан — шаг пропущен."
  [done]="Готово. Панель: https://%s"
  [conn_title]="Доступ к панели:"
  [conn_url]="  URL:    https://%s"
  [conn_local]="  локально: http://127.0.0.1:3002/"
  [conn_login]="  Логин:  %s"
  [conn_password]="  Пароль: %s"
  [conn_keep]="Пароль не менялся — админ уже был создан раньше."
  [conn_dry]="Пароль будет сгенерирован при установке."
  [conn_saved]="Пароль также записан в лог установки. Сменить можно в Настройках."
  [log_path]="Лог установки: %s"
  [ssh_unknown]="Не удалось определить SSH-порт — фаервол НЕ включён. Порты открой вручную или запусти с --skip-firewall."
  [caddy_foreign]="В /etc/caddy/Caddyfile есть чужая конфигурация — не трогаю. Перенеси её или поставь панель на отдельный сервер."
  [caddy_site]="В Caddyfile уже занят сайт на 80/443 — не трогаю."
  [xray_keep]="Уже стоит другая версия xray (%s) — оставляю. Обновить: --force."
  [err_download]="Не удалось скачать %s"
  [https_fail]="HTTPS не поднялся за 90 с. Последние строки журнала Caddy:"
  [https_warn]="Сертификат ещё не получен — Caddy повторит сам. Панель уже работает: http://127.0.0.1:3002/. Журнал: journalctl -u caddy -n 20"
  [update_hint]="Обновление: тот же скрипт повторно."
)
declare -A MSG_EN=(
  [welcome]="flypanel installer"
  [lang_ask]="Choose language [1] ru [2] en [3] de: "
  [need_root]="Root is required: sudo bash %s"
  [unsupported_os]="Supported: Ubuntu 24.04 and CentOS Stream 9/10 (x86_64). Found: %s. Manual install: docs/deploy-checklist.md"
  [no_systemd]="systemd not found — this is not a server/VPS."
  [container]="Looks like a container: systemd/journald will not work."
  [disk_low]="Not enough space on /: 2 GB needed, %s MB free."
  [port_busy]="Port %s is taken by a foreign process: %s"
  [ip_fail]="Could not detect the public IP. Pass it: --ip A.B.C.D"
  [ip_nat]="Note: IP %s may be private (NAT). Inbound VPN needs port forwarding."
  [dns_mismatch]="%s does not point to %s. Create the A record first (or --skip-dns)."
  [dns_ok]="DNS: %s -> %s"
  [q_domain]="Panel domain (A record must already point here): "
  [q_email]="E-mail for Let's Encrypt (may be empty): "
  [q_admin]="Admin login [admin]: "
  [already_running]="The installer is already running in another terminal. Wait for it to finish."
  [q_firewall]="Configure firewall (SSH port %s, 80, 443, 8443)? [Y/n]: "
  [confirm]="Install flypanel on %s for %s? [Y/n]: "
  [aborted]="Aborted."
  [step_update]="System update"
  [step_packages]="Packages"
  [step_user]="User and directories"
  [step_panel]="Panel binary"
  [step_xray]="xray-core"
  [step_env]="Environment file"
  [step_unit]="systemd unit"
  [step_firewall]="Firewall"
  [step_selinux]="SELinux"
  [step_caddy]="Caddy and HTTPS"
  [step_https]="HTTPS check"
  [step_admin]="Admin account"
  [ok]="ok"
  [skip]="skipped"
  [dry]="[dry-run]"
  [dry_preview]="Continuing in preview mode: nothing is changed."
  [admin_exists]="Admin already exists — step skipped."
  [done]="Done. Panel: https://%s"
  [conn_title]="Panel access:"
  [conn_url]="  URL:      https://%s"
  [conn_local]="  local:    http://127.0.0.1:3002/"
  [conn_login]="  Login:    %s"
  [conn_password]="  Password: %s"
  [conn_keep]="Password unchanged — the admin already existed."
  [conn_dry]="A password will be generated during install."
  [conn_saved]="The password is also in the install log. Change it in Settings."
  [log_path]="Install log: %s"
  [ssh_unknown]="Could not detect the SSH port — firewall NOT enabled. Open ports manually or use --skip-firewall."
  [caddy_foreign]="Foreign configuration in /etc/caddy/Caddyfile — leaving it alone."
  [caddy_site]="A site on 80/443 already exists in Caddyfile — leaving it alone."
  [xray_keep]="Another xray version is installed (%s) — keeping it. Update: --force."
  [err_download]="Failed to download %s"
  [https_fail]="HTTPS did not come up in 90 s. Last Caddy log lines:"
  [https_warn]="No certificate yet - Caddy keeps retrying. The panel already runs on http://127.0.0.1:3002/. Log: journalctl -u caddy -n 20"
  [update_hint]="Update: run the same script again."
)
declare -A MSG_DE=(
  [welcome]="flypanel installieren"
  [lang_ask]="Sprache wählen [1] ru [2] en [3] de: "
  [need_root]="Root erforderlich: sudo bash %s"
  [unsupported_os]="Unterstützt: Ubuntu 24.04 und CentOS Stream 9/10 (x86_64). Gefunden: %s. Manuell: docs/deploy-checklist.md"
  [no_systemd]="systemd fehlt — kein Server/VPS."
  [container]="Sieht nach einem Container aus: systemd/journald funktionieren nicht."
  [disk_low]="Zu wenig Platz auf /: 2 GB nötig, %s MB frei."
  [port_busy]="Port %s ist fremd belegt: %s"
  [ip_fail]="Öffentliche IP nicht erkannt. Angabe: --ip A.B.C.D"
  [ip_nat]="Hinweis: IP %s ist evtl. privat (NAT). Eingehendes VPN braucht Portweiterleitung."
  [dns_mismatch]="%s zeigt nicht auf %s. Zuerst A-Record anlegen (oder --skip-dns)."
  [dns_ok]="DNS: %s -> %s"
  [q_domain]="Panel-Domain (A-Record muss hierher zeigen): "
  [q_email]="E-Mail für Let's Encrypt (kann leer sein): "
  [q_admin]="Admin-Login [admin]: "
  [already_running]="Der Installer läuft bereits in einem anderen Terminal. Bitte warten."
  [q_firewall]="Firewall einrichten (SSH-Port %s, 80, 443, 8443)? [J/n]: "
  [confirm]="flypanel auf %s für %s installieren? [J/n]: "
  [aborted]="Abgebrochen."
  [step_update]="Systemaktualisierung"
  [step_packages]="Pakete"
  [step_user]="Benutzer und Verzeichnisse"
  [step_panel]="Panel-Binary"
  [step_xray]="xray-core"
  [step_env]="Umgebungsdatei"
  [step_unit]="systemd-Unit"
  [step_firewall]="Firewall"
  [step_selinux]="SELinux"
  [step_caddy]="Caddy und HTTPS"
  [step_https]="HTTPS-Prüfung"
  [step_admin]="Administrator"
  [ok]="fertig"
  [skip]="übersprungen"
  [dry]="[dry-run]"
  [dry_preview]="Weiter im Vorschaumodus: nichts wird geändert."
  [admin_exists]="Admin existiert bereits — Schritt übersprungen."
  [done]="Fertig. Panel: https://%s"
  [conn_title]="Panel-Zugang:"
  [conn_url]="  URL:        https://%s"
  [conn_local]="  lokal:      http://127.0.0.1:3002/"
  [conn_login]="  Login:      %s"
  [conn_password]="  Passwort:   %s"
  [conn_keep]="Passwort unverändert — der Admin existierte bereits."
  [conn_dry]="Das Passwort wird bei der Installation erzeugt."
  [conn_saved]="Das Passwort steht auch im Installationslog. Ändern in den Einstellungen."
  [log_path]="Installationslog: %s"
  [ssh_unknown]="SSH-Port nicht erkannt — Firewall NICHT aktiviert. Ports manuell öffnen oder --skip-firewall."
  [caddy_foreign]="Fremde Konfiguration in /etc/caddy/Caddyfile — unberührt."
  [caddy_site]="Site auf 80/443 existiert bereits in Caddyfile — unberührt."
  [xray_keep]="Andere xray-Version installiert (%s) — bleibt. Update: --force."
  [err_download]="Download fehlgeschlagen: %s"
  [https_fail]="HTTPS nach 90 s nicht erreichbar. Letzte Caddy-Logzeilen:"
  [https_warn]="Noch kein Zertifikat - Caddy versucht es weiter. Das Panel läuft bereits auf http://127.0.0.1:3002/. Log: journalctl -u caddy -n 20"
  [update_hint]="Update: Skript erneut ausführen."
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

fatal() {
  printf 'ERROR: %s\n' "$1" >&2
  exit 1
}

have() { command -v "$1" >/dev/null 2>&1; }

pkg_installed() {
  case $FAMILY in
    apt) dpkg -s "$1" >/dev/null 2>&1 ;;
    *) rpm -q "$1" >/dev/null 2>&1 ;;
  esac
}

# new_packages <pkg...>: prints the ones that are not installed yet.
new_packages() {
  local p out=""
  for p in "$@"; do pkg_installed "$p" || out="$out $p"; done
  printf '%s' "${out# }"
}

# run executes a command, or prints it in --dry-run mode.
run() {
  if $DRY_RUN; then
    printf '   %s %s\n' "$(t dry)" "$*"
    return 0
  fi
  printf '   $ %s\n' "$*"
  "$@"
}

parse_args() {
  while [ $# -gt 0 ]; do
    case $1 in
      --lang) LANG_SEL=${2:-}; shift 2 ;;
      --domain) DOMAIN=${2:-}; shift 2 ;;
      --email) EMAIL=${2:-}; shift 2 ;;
      --admin) ADMIN_NAME=${2:-}; shift 2 ;;
      --ip) IP=${2:-}; shift 2 ;;
      --yes|-y) ASSUME_YES=true; shift ;;
      --dry-run) DRY_RUN=true; shift ;;
      --skip-dns) SKIP_DNS=true; shift ;;
      --skip-firewall) SKIP_FIREWALL=true; shift ;;
      --no-update) SKIP_UPDATE=true; shift ;;
      --force) FORCE=true; shift ;;
      -h|--help) usage; exit 0 ;;
      *) fatal "unknown argument: $1 (see --help)" ;;
    esac
  done
}

usage() {
  cat <<'EOF'
usage: install.sh [--lang ru|en|de] [--domain D] [--email E] [--admin NAME]
                  [--ip A.B.C.D] [--yes] [--dry-run] [--skip-dns]
                  [--skip-firewall] [--no-update] [--force]
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
  if [ -t 0 ]; then
    printf '%s' "$(t lang_ask)"
    local answer
    read -r answer || answer=""
    case $answer in
      1|ru) LANG_SEL=ru ;;
      2|en) LANG_SEL=en ;;
      3|de) LANG_SEL=de ;;
    esac
  fi
}

detect_family() {
  [ -r "$OS_RELEASE_FILE" ] || { echo unsupported; return; }
  local ID="" VERSION_ID="" ID_LIKE=""
  # shellcheck disable=SC1090
  . "$OS_RELEASE_FILE" >/dev/null 2>&1 || true
  case "${ID:-}:${VERSION_ID:-}" in
    ubuntu:24.04) echo apt ;;
    centos:9|centos:9.*|centos:10|centos:10.*) echo dnf ;;
    *)
      case " ${ID_LIKE:-} " in
        *" rhel "*|*" centos "*) echo dnf ;;
        *" debian "*) echo unsupported ;;
        *) echo unsupported ;;
      esac
      ;;
  esac
}

valid_domain() {
  local d=$1
  [[ $d =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$ ]] || return 1
  case $d in example.com|*.example.com) return 1 ;; esac
  return 0
}

# port_free_or_ours: 0 when the port is free or owned by our own processes.
port_free_or_ours() {
  local port=$1 line proc
  line=$(ss -ltnp "sport = :$port" 2>/dev/null | sed -n '2p')
  [ -n "$line" ] || return 0
  proc=$(printf '%s' "$line" | sed -n 's/.*(("\([^"]*\)".*/\1/p')
  case $proc in
    flypanel|xray|caddy) return 0 ;;
  esac
  printf '%s' "${proc:-unknown}" > /tmp/.flypanel_port_owner
  return 1
}

detect_ssh_port() {
  local port=""
  # sshd -T needs root; the config file and a listening :22 probe cover the rest.
  port=$($SSHD_T_CMD 2>/dev/null | awk '/^port /{print $2; exit}') || port=""
  if [ -z "$port" ] && [ -r "${SSHD_CONFIG:-/etc/ssh/sshd_config}" ]; then
    port=$(awk 'tolower($1)=="port"{print $2; exit}' "${SSHD_CONFIG:-/etc/ssh/sshd_config}") || port=""
  fi
  if [ -z "$port" ]; then
    port=$(ss -ltnH 2>/dev/null | awk '{n=split($4,a,":"); if (a[n]=="22") {print 22; exit}}')
  fi
  printf '%s' "$port"
}

detect_ip() {
  local ip=""
  ip=$(curl -fsS --max-time 5 https://api.ipify.org 2>/dev/null) || ip=""
  [ -n "$ip" ] || ip=$(curl -fsS --max-time 5 https://ifconfig.co 2>/dev/null) || ip=""
  printf '%s' "$ip"
}

dns_points_here() {
  local domain=$1 want=$2 resolved=""
  resolved=$(getent ahostsv4 "$domain" 2>/dev/null | awk '{print $1; exit}') || resolved=""
  if [ -z "$resolved" ]; then
    resolved=$(curl -fsS --max-time 5 "https://dns.google/resolve?name=$domain&type=A" 2>/dev/null \
      | grep -o '"data":"[0-9.]*"' | sed 's/.*:"//; s/"//' | head -1) || resolved=""
  fi
  [ "$resolved" = "$want" ]
}

ask() {
  local prompt=$1 default=$2 answer
  if $ASSUME_YES; then printf '%s' "$default"; return; fi
  printf '%s' "$prompt" >&2
  read -r answer || answer=""
  [ -n "$answer" ] || answer=$default
  printf '%s' "$answer"
}

preflight() {
  local current
  if [ "$(id -u)" -ne 0 ] && ! $DRY_RUN; then
    sayf need_root "$0"; exit 1
  fi
  current=$(uname -m)
  if [ "$current" != "x86_64" ]; then
    # dry-run осматривает любую машину: проблемы здесь — предупреждение, а не выход.
    $DRY_RUN || $FORCE || fatal "$(printf "$(t unsupported_os)" "$current (arch)")"
  fi
  [ -d /run/systemd/system ] || {
    $DRY_RUN || $FORCE || fatal "$(t no_systemd)"
  }
  FAMILY=$(detect_family)
  if [ "$FAMILY" = unsupported ]; then
    if ! $FORCE; then
      local desc
      desc=$(. "$OS_RELEASE_FILE" 2>/dev/null && printf '%s %s' "${PRETTY_NAME:-unknown}" || echo unknown)
      $DRY_RUN || { sayf unsupported_os "$desc"; exit 1; }
      sayf unsupported_os "$desc"
      say dry_preview
    fi
    FAMILY=apt
  fi
  if [ -f /.dockerenv ] || grep -qa 'docker\|lxc' /proc/1/cgroup 2>/dev/null; then
    if ! $FORCE; then
      say container
      $DRY_RUN || exit 1
    fi
  fi
  local free
  free=$(df -Pk / | awk 'NR==2{print $4}')
  if [ "${free:-0}" -lt 2097152 ]; then
    sayf disk_low "$free"
    $DRY_RUN || exit 1
  fi
  local port
  for port in 80 443 8443 "$ACCESS_PORT" 10811; do
    if ! port_free_or_ours "$port"; then
      if $DRY_RUN; then
        sayf port_busy "$port" "$(cat /tmp/.flypanel_port_owner 2>/dev/null)"
      elif ! $FORCE; then
        sayf port_busy "$port" "$(cat /tmp/.flypanel_port_owner 2>/dev/null)"
        exit 1
      fi
    fi
  done
  if [ -z "$IP" ]; then
    IP=$(detect_ip)
    [ -n "$IP" ] || fatal "$(t ip_fail)"
  fi
  case $IP in
    10.*|192.168.*|172.1[6-9].*|172.2[0-9].*|172.3[01].*|127.*)
      sayf ip_nat "$IP" ;;
  esac
  SSH_PORT=$(detect_ssh_port)
}

# gen_password makes a random admin password: 16 alphanumerics from the kernel
# pool. The install log keeps a copy so the user can read it after the run.
gen_password() {
  head -c 256 /dev/urandom | LC_ALL=C tr -dc 'A-Za-z0-9' | cut -c1-16
}

# panel_admin_exists reports whether the installed panel already has an admin,
# so a re-run neither asks for a password nor overwrites the account.
panel_admin_exists() {
  local bin="$PANEL_DIR/bin/flypanel"
  [ -x "$bin" ] || return 1
  runuser -u flypanel -- env FLYPANEL_ENV="$PANEL_DIR/.env" "$bin" admin status >/dev/null 2>&1
}

collect_answers() {
  while [ -z "$DOMAIN" ]; do
    DOMAIN=$(ask "$(t q_domain)" "")
    valid_domain "$DOMAIN" || DOMAIN=""
  done
  if ! valid_domain "$DOMAIN"; then
    fatal "invalid domain: $DOMAIN"
  fi
  [ -n "$EMAIL" ] || EMAIL=$(ask "$(t q_email)" "")
  ADMIN_NAME=$(ask "$(t q_admin)" "$ADMIN_NAME")
  [ -n "$ADMIN_NAME" ] || ADMIN_NAME=admin
  local fw
  fw=$(ask "$(printf "$(t q_firewall)" "$SSH_PORT")" "y")
  case $fw in [nN]*) SKIP_FIREWALL=true ;; esac
  if ! $SKIP_DNS; then
    if dns_points_here "$DOMAIN" "$IP"; then
      sayf dns_ok "$DOMAIN" "$IP"
    elif $DRY_RUN; then
      sayf dns_mismatch "$DOMAIN" "$IP"
    elif $FORCE; then
      sayf dns_mismatch "$DOMAIN" "$IP"
    else
      sayf dns_mismatch "$DOMAIN" "$IP"
      exit 1
    fi
  fi
}

confirm() {
  $ASSUME_YES && return 0
  local answer
  answer=$(ask "$(printf "$(t confirm)" "$IP" "$DOMAIN")" "y")
  case $answer in
    [nN]*) say aborted; exit 1 ;;
  esac
}

update_system() {
  step step_update
  if $SKIP_UPDATE; then say skip; return; fi
  # Полное обновление до установки: иначе свежая машина остаётся с ядром и
  # openssl годовой давности. Отключается флагом --no-update.
  if [ "$FAMILY" = apt ]; then
    run env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a apt-get update -qq
    run env DEBIAN_FRONTEND=noninteractive NEEDRESTART_MODE=a \
      apt-get -y -o Dpkg::Options::=--force-confold upgrade
  else
    run dnf -y upgrade --refresh
  fi
  ok
}

install_packages() {
  step step_packages
  local pkgs
  if [ "$FAMILY" = apt ]; then
    pkgs="curl ca-certificates unzip ufw iproute2"
  else
    pkgs="curl ca-certificates unzip firewalld policycoreutils-python-utils iproute"
  fi
  # Remember only what we add: the uninstaller must not strip packages that
  # were already on the machine.
  PKG_NEW=$(new_packages $pkgs)
  if $DRY_RUN; then printf '   %s packages via %s\n' "$(t dry)" "$FAMILY"; return; fi
  printf '   -> %s\n' "$pkgs"
  if [ "$FAMILY" = apt ]; then
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y $pkgs
  else
    dnf install -y $pkgs
    systemctl enable --now firewalld
  fi
  ok
}

create_user_dirs() {
  step step_user
  run useradd --system --home-dir "$PANEL_DIR" --shell /usr/sbin/nologin flypanel 2>/dev/null || true
  run mkdir -p "$PANEL_DIR/bin" "$PANEL_DIR/data" "$PANEL_DIR/logs" "$PANEL_DIR/backups"
  if [ -n "$PKG_NEW" ] && ! $DRY_RUN; then
    printf '%s\n' $PKG_NEW > "$PANEL_DIR/installed-packages"
  fi
  ok
}

download() {
  local url=$1 dest=$2
  printf '   <- %s\n' "$url"
  # Медленный канал не должен валить установку: 10 минут, 3 попытки, докачка.
  # -C - продолжает недокачанный файл, sha256 всё равно проверяется после.
  local opts=(-fL --connect-timeout 15 --max-time 600 --retry 3 --retry-delay 3
    --retry-all-errors -C - --show-error)
  if [ -t 2 ]; then opts+=(--progress-bar); else opts+=(--silent); fi
  curl "${opts[@]}" "$url" -o "$dest" || fatal "$(printf "$(t err_download)" "$url")"
  printf '   -> %s (%s)\n' "$dest" "$(du -h "$dest" | cut -f1)"
}

install_panel() {
  step step_panel
  local tmp=${PANEL_DIR}/backups/.install.$$
  run mkdir -p "$PANEL_DIR/backups"
  if $DRY_RUN; then
    printf '   %s download flypanel-linux-amd64 from releases/latest\n' "$(t dry)"
    return
  fi
  mkdir -p "$tmp"
  download "https://github.com/$REPO/releases/latest/download/flypanel-linux-amd64" "$tmp/flypanel"
  download "https://github.com/$REPO/releases/latest/download/sha256sums.txt" "$tmp/sha256sums.txt"
  ( cd "$tmp" && mv flypanel flypanel-linux-amd64 \
    && grep ' flypanel-linux-amd64$' sha256sums.txt > sums.txt \
    && sha256sum -c sums.txt ) || fatal "sha256 mismatch for flypanel-linux-amd64"
  install -o flypanel -g flypanel -m 755 "$tmp/flypanel-linux-amd64" "$PANEL_DIR/bin/flypanel.new"
  mv -f "$PANEL_DIR/bin/flypanel.new" "$PANEL_DIR/bin/flypanel"
  rm -rf "$tmp"
  printf '   -> %s\n' "$PANEL_DIR/bin/flypanel"
  ok
}

install_xray() {
  step step_xray
  if $DRY_RUN; then printf '   %s xray %s\n' "$(t dry)" "$XRAY_VERSION"; return; fi
  if [ -x "$PANEL_DIR/bin/xray" ] && ! $FORCE; then
    local current
    current=$("$PANEL_DIR/bin/xray" version 2>/dev/null | awk '/^Xray /{print $2; exit}') || current=""
    if [ -n "$current" ] && [ "$current" != "v$XRAY_VERSION" ] && [ "$current" != "$XRAY_VERSION" ]; then
      sayf xray_keep "$current"
      return
    fi
  fi
  local tmp=${PANEL_DIR}/backups/.xray.$$
  mkdir -p "$tmp"
  local base="https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VERSION}"
  download "$base/Xray-linux-64.zip" "$tmp/xray.zip"
  download "$base/Xray-linux-64.zip.dgst" "$tmp/xray.zip.dgst"
  local want got
  want=$(grep -o 'SHA2-256= *[0-9a-f]*' "$tmp/xray.zip.dgst" | awk '{print $2}' | head -1)
  got=$(sha256sum "$tmp/xray.zip" | awk '{print $1}')
  [ -n "$want" ] && [ "$want" = "$got" ] || fatal "sha256 mismatch for xray"
  unzip -oq "$tmp/xray.zip" xray geoip.dat geosite.dat -d "$tmp"
  install -o flypanel -g flypanel -m 755 "$tmp/xray" "$PANEL_DIR/bin/xray"
  install -o flypanel -g flypanel -m 644 "$tmp/geoip.dat" "$PANEL_DIR/bin/geoip.dat"
  install -o flypanel -g flypanel -m 644 "$tmp/geosite.dat" "$PANEL_DIR/bin/geosite.dat"
  rm -rf "$tmp"
  printf '   -> %s/bin/{xray,geoip.dat,geosite.dat}\n' "$PANEL_DIR"
  ok
}

env_file_write() {
  local path=$1
  if [ -f "$path" ]; then
    cp -p "$path" "$path.bak-$(date +%Y%m%d%H%M)"
  fi
  local old_umask
  old_umask=$(umask)
  umask 077
  cat > "$path" <<EOF
LISTEN_ADDR=127.0.0.1:${ACCESS_PORT}
DATA_DIR=${PANEL_DIR}/data
XRAY_BIN=${PANEL_DIR}/bin/xray
ENTRY_IP=${IP}
EGRESS_IP=
PANEL_DOMAIN=${DOMAIN}
POLL_INTERVAL_SEC=30
COOKIE_SECURE=true
EOF
  chmod 600 "$path" 2>/dev/null || true
  umask "$old_umask"
}

install_env() {
  step step_env
  if $DRY_RUN; then
    printf '   %s write %s/.env\n' "$(t dry)" "$PANEL_DIR"
    return
  fi
  env_file_write "$PANEL_DIR/.env"
  printf '   -> %s/.env\n' "$PANEL_DIR"
  chown flypanel:flypanel "$PANEL_DIR/.env"
  chown -R flypanel:flypanel "$PANEL_DIR/data" "$PANEL_DIR/logs" "$PANEL_DIR/backups"
  ok
}

install_unit() {
  step step_unit
  local unit=/etc/systemd/system/flypanel.service
  if $DRY_RUN; then printf '   %s write %s\n' "$(t dry)" "$unit"; return; fi
  cat > "$unit" <<EOF
[Unit]
Description=flypanel VPN panel
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=flypanel
Group=flypanel
WorkingDirectory=${PANEL_DIR}
Environment=FLYPANEL_ENV=${PANEL_DIR}/.env
ExecStart=${PANEL_DIR}/bin/flypanel serve
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
ProtectSystem=strict
PrivateTmp=true
ReadWritePaths=${PANEL_DIR}
SupplementaryGroups=systemd-journal

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable flypanel >/dev/null 2>&1 || true
  systemctl restart flypanel
  printf '   -> %s\n' "$unit"
  ok
}

firewall_allowed() {
  [ -n "${SSH_PORT:-}" ] && [ "$1" != "no-ssh" ]
}

configure_firewall() {
  step step_firewall
  if $SKIP_FIREWALL; then say skip; return; fi
  if ! firewall_allowed "$SSH_PORT"; then
    say ssh_unknown
    $DRY_RUN || FIREWALL_FAILED=true
    return
  fi
  if $DRY_RUN; then
    printf '   %s open %s/tcp 80/tcp 443/tcp 8443/tcp via %s\n' "$(t dry)" "$SSH_PORT" "$FAMILY"
    return
  fi
  if [ "$FAMILY" = apt ]; then
    run ufw allow "${SSH_PORT}/tcp"
    run ufw allow 80/tcp
    run ufw allow 443/tcp
    run ufw allow 8443/tcp
    run ufw --force enable
  else
    run firewall-cmd --permanent --add-port="${SSH_PORT}/tcp"
    run firewall-cmd --permanent --add-port=80/tcp
    run firewall-cmd --permanent --add-port=443/tcp
    run firewall-cmd --permanent --add-port=8443/tcp
    run firewall-cmd --reload
  fi
  ok
}

configure_selinux() {
  [ "$FAMILY" = dnf ] || return 0
  step step_selinux
  if ! have getenforce || [ "$(getenforce 2>/dev/null)" != "Enforcing" ]; then
    say skip
    return
  fi
  run semanage port -a -t http_port_t -p tcp 8443 2>/dev/null || run semanage port -m -t http_port_t -p tcp 8443
  run semanage port -a -t http_port_t -p tcp "$ACCESS_PORT" 2>/dev/null || run semanage port -m -t http_port_t -p tcp "$ACCESS_PORT"
  run systemctl restart caddy flypanel
  ok
}

managed_block() {
  cat <<EOF
# flypanel-managed begin
$DOMAIN {
	log
	reverse_proxy 127.0.0.1:${ACCESS_PORT}
}
# flypanel-managed end
EOF
}

# caddyfile_apply rewrites only our own block: global { metrics } stays when
# present, a foreign site or a foreign global block aborts untouched.
caddyfile_apply() {
  local path=$1
  local tmp
  tmp=$(mktemp)
  if [ ! -s "$path" ]; then
    { printf '{\n\tmetrics\n}\n\n'; managed_block; } > "$tmp"
    mv "$tmp" "$path"
    chmod 644 "$path"
    return 0
  fi
  if ! awk -v out="$tmp" '
    BEGIN { depth=0; global_done=0; ing=0; global="" ; rest="" }
    /^# flypanel-managed begin$/ { ing=1; next }
    /^# flypanel-managed end$/ { ing=0; next }
    ing==1 { next }
    { lines[NR]=$0 }
    END {
      n=NR; i=1
      while (i<=n && (lines[i] ~ /^[[:space:]]*$/ || lines[i] ~ /^[[:space:]]*#/)) i++
      if (i<=n && lines[i] ~ /^\{[[:space:]]*$/) {
        global=lines[i] "\n"; depth=1; i++
        while (i<=n && depth>0) {
          global=global lines[i] "\n"
          if (lines[i] ~ /\{/) depth++
          if (lines[i] ~ /\}/) depth--
          i++
        }
        global_done=1
      }
      rest=""
      for (; i<=n; i++) {
        if (lines[i] ~ /^[[:space:]]*$/) continue
        if (lines[i] ~ /^[[:space:]]*#/) continue
        rest=rest lines[i] "\n"
      }
      if (!global_done) global="{\n\tmetrics\n}\n"
      if (global_done && global !~ /metrics/) exit 3
      if (rest != "") exit 4
      printf "%s\n", global > out
      close(out)
    }
  ' "$path"; then
    rm -f "$tmp"
    return 1
  fi
  { cat "$tmp"; managed_block; } > "$tmp.full"
  mv "$tmp.full" "$path"
  chmod 644 "$path"
  rm -f "$tmp"
  return 0
}

install_caddy() {
  step step_caddy
  local dir=/etc/caddy
  if $DRY_RUN; then
    printf '   %s install caddy %s and %s/Caddyfile\n' "$(t dry)" "$CADDY_VERSION" "$dir"
    return
  fi
  local tmp
  tmp=$(mktemp -d)
  local base="https://github.com/caddyserver/caddy/releases/download/v${CADDY_VERSION}"
  download "$base/caddy_${CADDY_VERSION}_linux_amd64.tar.gz" "$tmp/caddy_${CADDY_VERSION}_linux_amd64.tar.gz"
  download "$base/caddy_${CADDY_VERSION}_checksums.txt" "$tmp/sums.txt"
  ( cd "$tmp" && grep " caddy_${CADDY_VERSION}_linux_amd64.tar.gz$" sums.txt > one.txt \
    && sha512sum -c one.txt >/dev/null ) || fatal "sha512 mismatch for caddy"
  tar -xzf "$tmp/caddy_${CADDY_VERSION}_linux_amd64.tar.gz" -C "$tmp" caddy
  install -m 755 "$tmp/caddy" /usr/local/bin/caddy
  rm -rf "$tmp"

  id caddy >/dev/null 2>&1 || useradd --system --home-dir /var/lib/caddy --shell /usr/sbin/nologin caddy
  mkdir -p "$dir" /var/lib/caddy
  chmod 755 "$dir"
  chown caddy:caddy /var/lib/caddy

  if [ -f "$dir/Caddyfile" ]; then
    cp -p "$dir/Caddyfile" "$dir/Caddyfile.bak-$(date +%Y%m%d%H%M)"
  fi
  caddyfile_apply "$dir/Caddyfile" || { say caddy_foreign; exit 1; }

  cat > /etc/systemd/system/caddy.service <<'EOF'
[Unit]
Description=Caddy web server
After=network-online.target
Wants=network-online.target

[Service]
Type=notify
User=caddy
Group=caddy
ExecStart=/usr/local/bin/caddy run --environ --config /etc/caddy/Caddyfile
ExecReload=/usr/local/bin/caddy reload --config /etc/caddy/Caddyfile --force
TimeoutStopSec=5s
LimitNOFILE=1048576
PrivateTmp=true
ProtectSystem=full
AmbientCapabilities=CAP_NET_BIND_SERVICE
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  /usr/local/bin/caddy validate --config "$dir/Caddyfile" >/dev/null || fatal "caddy config invalid"
  systemctl enable caddy >/dev/null 2>&1 || true
  systemctl restart caddy
  printf '   -> /usr/local/bin/caddy\n   -> %s/Caddyfile\n' "$dir"
  ok
}

wait_https() {
  step step_https
  if $DRY_RUN; then printf '   %s wait for https://%s\n' "$(t dry)" "$DOMAIN"; return; fi
  local i body
  for i in $(seq 1 45); do
    body=$(curl -fsS --max-time 5 "https://$DOMAIN/" 2>/dev/null) || body=""
    if printf '%s' "$body" | grep -q 'assets/index-'; then ok; return; fi
    sleep 2
  done
  # Панель может быть готова, а сертификат ещё нет: лимит Let's Encrypt,
  # DNS-кеш, медленный ACME. Это не повод валить установку.
  if [ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 3 http://127.0.0.1:3002/ || true)" = 200 ]; then
    say https_warn
    journalctl -u caddy -n 8 --no-pager 2>/dev/null || true
    HTTPS_PENDING=1
    return 0
  fi
  say https_fail
  journalctl -u caddy -n 20 --no-pager 2>/dev/null || true
  exit 1
}

admin_step() {
  step step_admin
  local bin="$PANEL_DIR/bin/flypanel"
  if $DRY_RUN; then printf '   %s flypanel admin status\n' "$(t dry)"; return; fi
  if runuser -u flypanel -- env FLYPANEL_ENV="$PANEL_DIR/.env" "$bin" admin status >/dev/null 2>&1; then
    say admin_exists
    return
  fi
  ADMIN_PASSWORD=$(gen_password)
  # Flags before username: the flag package stops parsing at the first
  # positional argument.
  printf '%s\n' "$ADMIN_PASSWORD" |
    runuser -u flypanel -- env FLYPANEL_ENV="$PANEL_DIR/.env" "$bin" admin init --password-stdin "$ADMIN_NAME" || {
    printf 'admin init failed\n' >&2
    ADMIN_FAILED=true
    return
  }
  ADMIN_CREATED=1
  ok
}

summary() {
  printf '\n'
  sayf done "$DOMAIN"
  # Credentials come last: the user scrolls here once the steps are done.
  say conn_title
  sayf conn_url "$DOMAIN"
  if [ "$HTTPS_PENDING" = 1 ]; then say conn_local; fi
  sayf conn_login "$ADMIN_NAME"
  if $DRY_RUN; then
    say conn_dry
  elif [ "$ADMIN_CREATED" = 1 ]; then
    sayf conn_password "$ADMIN_PASSWORD"
    say conn_saved
  else
    say conn_keep
  fi
  ADMIN_PASSWORD=""
  sayf log_path "$INSTALL_LOG"
  say update_hint
  if [ "$HTTPS_PENDING" = 1 ]; then say https_warn; fi
  if ! $DRY_RUN && { $FIREWALL_FAILED || $ADMIN_FAILED; }; then
    printf '\nСмотри предупреждения выше.\n'
    exit 1
  fi
}

main() {
  parse_args "$@"
  pick_lang
  # Root before the log is opened: an unprivileged run must not print tee errors.
  if [ "$(id -u)" -ne 0 ] && ! $DRY_RUN; then
    sayf need_root "$0"; exit 1
  fi
  # One installer at a time: a second run scrambles the log and races on apt.
  if ! $DRY_RUN; then
    exec 9>/run/lock/flypanel-install.lock
    flock -n 9 || { say already_running; exit 1; }
  fi
  echo "$(t welcome)"
  if ! $DRY_RUN; then
    exec > >(tee -a "$INSTALL_LOG") 2>&1
  fi
  preflight
  collect_answers
  confirm
  update_system
  install_packages
  create_user_dirs
  install_panel
  install_xray
  install_env
  install_unit
  configure_firewall
  configure_selinux
  install_caddy
  wait_https
  admin_step
  summary
}

[[ ${INSTALL_LIB_ONLY:-0} == 1 ]] || main "$@"