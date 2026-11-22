# Установщик flypanel — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** один `install.sh`, который ставит панель, xray и Caddy на чистый Ubuntu 24.04 или CentOS Stream 9/10 и доводит до рабочего HTTPS-адреса.

**Architecture:** bash-скрипт с языковым выбором на первом экране, общим ядром (юзер, каталоги, бинарь, xray, `.env`, systemd-юнит, Caddy) и двумя ветками по семейству ОС (apt/ufw, dnf/firewalld/selinux). Панель ставится из GitHub Releases; в бинарь добавляется `admin status`, чтобы установщик не тащил `sqlite3`.

**Tech Stack:** bash 5, bash-функции и `INSTALL_LIB_ONLY=1` для тестов, GitHub Actions, Go 1.27 (только подкоманда `admin status`).

**Spec:** `docs/superpowers/specs/2026-11-22-installer-design.md`

## Global Constraints

- Целевые ОС: Ubuntu 24.04 и CentOS Stream 9/10, `x86_64`. Прочие — отказ, `--force` без гарантий.
- Скрипт работает от root, не удаляет `data/panel.db` и `data/backups` никогда.
- `/etc/caddy/Caddyfile` и `.env` при перезаписи — бэкап `*.bak-YYYYMMDDHHMM`.
- Свои блоки в Caddyfile — под маркерами `# flypanel-managed begin` / `# flypanel-managed end`.
- Версии закреплены: `XRAY_VERSION=26.11.9` (ассет `Xray-linux-64.zip`, проверка `.zip.dgst`), `CADDY_VERSION=2.11.9` (проверка SHA512 по `caddy_2.11.9_checksums.txt`).
- Все сообщения пользователю — через словари `TXT_RU/TXT_EN/TXT_DE`, без gettext и внешних файлов.
- Языки/флаги: `--lang ru|en|de`, `--domain`, `--email`, `--admin`, `--ip`, `--yes`, `--dry-run`, `--skip-dns`, `--skip-firewall`, `--force`.
- Ничего не пишется вне `/opt/flypanel`, `/etc/systemd/system`, `/etc/caddy`, `/usr/local/bin`, `/var/log`, `/etc/os-release`-читается.

## Review Focus

- Повторный запуск на работающей панели: порты 3002/8443/10811 заняты нашими же процессами — префлайт обязан пройти (тест `port_free_or_ours` в Task 2).
- Порт SSH не определён: фаервол не включается, скрипт выходит (тест `firewall_allowed` в Task 4).
- `--dry-run` не создаёт ни одного файла (проверка в Task 9 на контейнере).
- Чужой Caddyfile или чужой сайт на 80/443 не перезаписывается (тесты `caddyfile_apply` в Task 5).
- Смена домена на повторном запуске заменяет управляемый блок, а не добавляет второй (тест `caddyfile_apply`).
- Админ уже есть → шаг admin пропускается, `admin status` возвращает 0 (Task 1 и Task 6).

---

### Task 1: `flypanel admin status`

**Files:**
- Modify: `cmd/flypanel/admin.go`
- Test: `cmd/flypanel/admin_test.go` (создать)

**Interfaces:**
- Consumes: `store.Open(dir)`, `(*store.Store).AdminGet() (*store.Admin, error)`, `store.ErrNotFound`.
- Produces: `func adminStatus(dir string) (bool, error)` — `true`, если админ есть; CLI `flypanel admin status [--data-dir DIR]` печатает `admin exists` и выходит 0, иначе `no admin` и выходит 1.

- [ ] **Step 1: Написать падающий тест**

```go
package main

import (
	"testing"

	"flypanel/internal/auth"
	"flypanel/internal/store"
)

func TestAdminStatus(t *testing.T) {
	dir := t.TempDir()
	exists, err := adminStatus(dir)
	if err != nil {
		t.Fatalf("adminStatus(empty): %v", err)
	}
	if exists {
		t.Fatal("adminStatus(empty) = true, want false")
	}

	st, err := store.Open(dir)
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	if err := auth.AdminInit(st, "admin", "FlyPanel-2026x"); err != nil {
		t.Fatalf("AdminInit(): %v", err)
	}
	st.Close()

	exists, err = adminStatus(dir)
	if err != nil {
		t.Fatalf("adminStatus(seeded): %v", err)
	}
	if !exists {
		t.Fatal("adminStatus(seeded) = false, want true")
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `go test ./cmd/flypanel/ -run TestAdminStatus`
Expected: FAIL — `undefined: adminStatus`.

- [ ] **Step 3: Реализовать**

В `cmd/flypanel/admin.go` развести `adminCommand` на подкоманды и добавить функцию. Путь к данным — из `config.Load()` + `--data-dir`, как в `init`.

```go
func adminStatus(dir string) (bool, error) {
	st, err := store.Open(dir)
	if err != nil {
		return false, err
	}
	defer st.Close()
	_, err = st.AdminGet()
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
```

CLI-ветка `status`: свой `flag.NewFlagSet("admin status")`, печать `admin exists` / `no admin`, `os.Exit(0)` / `os.Exit(1)`; `usage()` в `main.go` дополнить строкой `admin status`.

- [ ] **Step 4: Прогнать тест**

Run: `go test ./cmd/flypanel/ -run TestAdminStatus -v`
Expected: PASS.

- [ ] **Step 5: Коммит**

```bash
git add cmd/flypanel/admin.go cmd/flypanel/admin_test.go cmd/flypanel/main.go
git commit -m "feat(admin): report whether an admin account exists"
```

---

### Task 2: каркас скрипта, язык, префлайт

**Files:**
- Create: `scripts/install.sh`
- Test: `scripts/install_test.sh`

**Interfaces:**
- Consumes: ничего из Go.
- Produces (функции, доступные при `INSTALL_LIB_ONLY=1`): `t <K>`; `detect_family` → печатает `apt|dnf|unsupported`, читает `/etc/os-release`; `valid_domain <d>` → 0/1; `port_free_or_ours <port>` → 0, если порт свободен или занят процессом из `flypanel|xray|caddy`; `detect_ssh_port` → печатает порт или пусто; `have <cmd>`; `run <cmd...>` (в dry-run печатает, иначе исполняет).

- [ ] **Step 1: Написать падающий тест-харнесс**

`scripts/install_test.sh`: `set -euo pipefail`, `INSTALL_LIB_ONLY=1 source scripts/install.sh`, счётчик провалов, функции:

- `detect_family` на подставленном `/etc/os-release` (переменная `OS_RELEASE_FILE`) → `ubuntu`+`24.04` = `apt`; `centos`+`9` = `dnf`; `debian`+`12` = `unsupported`; `centos`+`7` = `unsupported`.
- `valid_domain panel.example.org` = 0; `valid_domain "panel"` = 1; `valid_domain "example.com"` = 1; `valid_domain "<panel>"` = 1.
- `port_free_or_ours`: занять порт `python3 -c 'import socket,time; s=socket.socket(); s.bind(("127.0.0.1",P)); s.listen(); time.sleep(2)'` — чужой процесс → 1; порт без слушателя → 0. Тест на «наш процесс» не пишем: подделка имени процесса хрупкая, поведение покрывается реальным повторным запуском в Task 10.
- `detect_ssh_port` c `SSHD_T_CMD=/bin/false` (пустой вывод) → пустая строка.

- [ ] **Step 2: Прогнать — упасть**

Run: `bash scripts/install_test.sh`
Expected: FAIL — `scripts/install.sh: No such file or directory`.

- [ ] **Step 3: Реализовать каркас**

`scripts/install.sh`:

```bash
#!/usr/bin/env bash
set -euo pipefail

INSTALL_LOG=/var/log/flypanel-install.log
XRAY_VERSION=26.11.9
CADDY_VERSION=2.11.9
OS_RELEASE_FILE=${OS_RELEASE_FILE:-/etc/os-release}
SSHD_T_CMD=${SSHD_T_CMD:-sshd -T}

# --- i18n: три параллельных массива, индексные константы ---
TXT_RU=(...); TXT_EN=(...); TXT_DE=(...)
K_WELCOME=0; ...
t() { local k=$1; case $LANG_SEL in ru) printf '%s' "${TXT_RU[$k]}" ;; ... esac; }

detect_family() {
  [ -r "$OS_RELEASE_FILE" ] || { echo unsupported; return; }
  . "$OS_RELEASE_FILE"
  case "${ID:-}:${VERSION_ID:-}" in
    ubuntu:24.04) echo apt ;;
    centos:9|centos:10|centos:9.*|centos:10.*) echo dnf ;;
    *) echo unsupported ;;
  esac
}

valid_domain() { [[ $1 =~ ^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$ ]] && [[ $1 != example.com ]] && [[ $1 != *'<'* ]]; }

have() { command -v "$1" >/dev/null 2>&1; }

run() { if $DRY_RUN; then printf '  [dry-run] %s\n' "$*"; else "$@"; fi; }

port_free_or_ours() { ... ss -ltnp ... }

detect_ssh_port() { $SSHD_T_CMD 2>/dev/null | awk '/^port /{print $2; exit}'; }

main() { ... }   # ниже, Task 3+
[[ ${INSTALL_LIB_ONLY:-0} == 1 ]] || main "$@"
```

Плюс реальный `main`-скелет: `parse_args`, выбор языка (дефолт из `$LANG`), `preflight` (root, `x86_64`, systemd, family, контейнер, диск, порты, IP, DNS, ssh-порт), экран подтверждения. Тексты — во всех трёх словарях.

- [ ] **Step 4: Прогнать тесты**

Run: `bash scripts/install_test.sh`
Expected: `all checks passed`.

- [ ] **Step 5: Коммит**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "feat(installer): skeleton, i18n and preflight"
```

---

### Task 3: общее ядро установки

**Files:**
- Modify: `scripts/install.sh`
- Test: `scripts/install_test.sh` (проверки `env_file_write`)

**Interfaces:**
- Consumes: `run`, `have`, `t`, `FAMILY`, `IP`, `DOMAIN`, `ADMIN_NAME`, `EMAIL`.
- Produces: `env_file_write <path>` → пишет `.env` c бэкапом; `install_packages`; `create_user_dirs`; `install_panel`; `install_xray`; `install_unit`; `wait_https <domain>` → 0/1.

- [ ] **Step 1: Тест на `.env`**

В `scripts/install_test.sh`: `env_file_write` в пустой каталог создаёт файл с `LISTEN_ADDR=127.0.0.1:3002`, `DATA_DIR=<dir>/data`, `ENTRY_IP=203.0.113.9`, `PANEL_DOMAIN=panel.example.org`, `COOKIE_SECURE=true`, права `600`; повторный вызов создаёт `*.bak-*` и не теряет старые значения вне перечисленных ключей.

Run: `bash scripts/install_test.sh` → FAIL (функция не определена).

- [ ] **Step 2: Реализовать шаги**

- `install_packages`: Ubuntu `apt-get update` + `curl ca-certificates unzip ufw iproute2`; CentOS `dnf install -y curl ca-certificates unzip firewalld policycoreutils-python-utils iproute` + `systemctl enable --now firewalld`.
- `create_user_dirs`: `useradd --system --home-dir /opt/flypanel --shell /usr/sbin/nologin flypanel` (если нет), `mkdir -p bin data logs backups`.
- `install_panel`: скачать `https://github.com/dev-by-dev/fly-vpn-panel/releases/latest/download/flypanel-linux-amd64` и `sha256sums.txt` в `mktemp -d`, `sha256sum -c` по строке с этим файлом, установить в `bin/flypanel.new`, `chown flypanel:flypanel`, `chmod 755`, `mv` поверх `bin/flypanel`.
- `install_xray`: `Xray-linux-64.zip` + `.zip.dgst` из `https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VERSION}/`, сверить `SHA2-256=`, распаковать `xray geoip.dat geosite.dat` в `bin/`; при существующем `bin/xray` другой версии — предупредить и не трогать без `--force`.
- `env_file_write`: бэкап `.bak-$(date +%Y%m%d%H%M)`, запись ровно десяти ключей из спеки, `chmod 600`, `chown flypanel:flypanel`.
- `install_unit`: юнит из чеклиста (`PrivateTmp=true`, `ProtectSystem=strict`, `ReadWritePaths=/opt/flypanel`, `SupplementaryGroups=systemd-journal`, `Restart=on-failure`), `systemctl daemon-reload`, `enable`, `restart` (не `--now`, панель поднимаем после `.env`).
- `wait_https`: до 90 с `curl -fsS "https://$1/"` с проверкой `assets/index-`.

- [ ] **Step 3: Прогнать тесты**

Run: `bash scripts/install_test.sh`
Expected: PASS, включая `env_file_write`.

- [ ] **Step 4: Коммит**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "feat(installer): panel, xray, env and unit steps"
```

---

### Task 4: фаервол и SELinux

**Files:**
- Modify: `scripts/install.sh`
- Test: `scripts/install_test.sh` (проверка `firewall_allowed`)

**Interfaces:**
- Consumes: `FAMILY`, `SSH_PORT`, `ENABLE_FIREWALL`.
- Produces: `firewall_allowed <port>` → 0, если есть ssh-порт; `configure_firewall`; `configure_selinux`.

- [ ] **Step 1: Тест**

`firewall_allowed` при `SSH_PORT=""` → 1 (нельзя включать), при `SSH_PORT=2222` → 0. `configure_firewall` при `SSH_PORT=""` не вызывает `run` (перехватить `run` подменой в тесте: `run() { echo "RUN $*"; }`).

- [ ] **Step 2: Реализовать**

`configure_firewall`: Ubuntu — `ufw allow "$SSH_PORT/tcp"`, затем 80, 443, 8443, `ufw --force enable`; CentOS — `firewall-cmd --permanent --add-port="$SSH_PORT/tcp"` и те же порты, `--reload`. Порядок: SSH первым, включение последним. Если `detect_ssh_port` пуст — предупредить, напечатать команды вручную, выйти (или продолжить при `--skip-firewall`).

`configure_selinux`: только если `getenforce` = `Enforcing`; `semanage port -a -t http_port_t -p tcp 8443`, `3002` (при занятой метке — `-m`), `systemctl restart caddy flypanel`; ошибки — предупреждение, не выход.

- [ ] **Step 3: Тесты**

Run: `bash scripts/install_test.sh` → PASS.

- [ ] **Step 4: Коммит**

```bash
git commit -am "feat(installer): firewall and selinux branches"
```

---

### Task 5: Caddy с маркерами

**Files:**
- Modify: `scripts/install.sh`
- Test: `scripts/install_test.sh` (тесты `caddyfile_apply`)

**Interfaces:**
- Produces: `caddyfile_apply <path> <domain>` → 0 при успехе; 1 с сообщением, если конфиг чужой; `install_caddy`.

- [ ] **Step 1: Тесты (RED)**

`caddyfile_apply` на пустом файле → блок с маркерами, `{ metrics }` и `reverse_proxy 127.0.0.1:3002`; повторный вызов с другим доменом → один управляемый блок, внутри новый домен, старый отсутствует; файл с чужим сайтом на `:80` и без маркеров → 1, файл не изменён; файл с чужим глобальным блоком `{ email a@b.c }` → 1.

- [ ] **Step 2: Реализовать**

Скачать `caddy_${CADDY_VERSION}_linux_amd64.tar.gz` + `caddy_${CADDY_VERSION}_checksums.txt` (SHA512), установить в `/usr/local/bin/caddy`; юзер `caddy`, каталоги; юнит `caddy.service`; `caddyfile_apply` (awk/python-free: `sed`/heredoc, маркеры обязательны); `caddy validate --config /etc/caddy/Caddyfile`; `systemctl enable caddy` + `restart` (или `reload`, если активен).

- [ ] **Step 3: Тесты**

Run: `bash scripts/install_test.sh` → PASS (4 кейса Caddyfile).

- [ ] **Step 4: Коммит**

```bash
git commit -am "feat(installer): caddy with managed config markers"
```

---

### Task 6: админ, сводка, порядок шагов

**Files:**
- Modify: `scripts/install.sh`

**Interfaces:**
- Consumes: `flypanel admin status` (Task 1).
- Produces: `admin_step`; финальную сводку.

- [ ] **Step 1: Реализовать**

`admin_step`: `runuser -u flypanel -- env FLYPANEL_ENV=/opt/flypanel/.env /opt/flypanel/bin/flypanel admin status` → код 0 — печать «админ уже есть», пропуск; иначе, если не `--yes`, `runuser -u flypanel -- env ... admin init` (терминал остаётся пользователю); `--yes` — пропуск с напоминанием. Провал `admin init` — предупреждение, код выхода скрипта 1 в конце.

Порядок `main`: args → язык → preflight → вопросы → подтверждение → packages → user/dirs → panel → xray → env → chown → unit → firewall → selinux → caddy → wait_https → admin → summary.

- [ ] **Step 2: Проверить целиком в dry-run**

Run: `sudo bash scripts/install.sh --dry-run --force --domain panel.example.org --admin admin --lang ru`
Expected: все 13 шагов напечатаны в правильном порядке, ни одного изменения на диске (`ls /opt/flypanel` → нет такого каталога), код 0.

- [ ] **Step 3: Коммит**

```bash
git commit -am "feat(installer): admin handoff and summary"
```

---

### Task 7: документация

**Files:**
- Modify: `README.md`, `docs/deploy-checklist.md`

- [ ] **Step 1:** В `README.md` — раздел «Установка»: одна команда скачивания скрипта (не `curl | bash`), поддерживаемые ОС, что спросит, где лог, как обновлять.
- [ ] **Step 2:** В `docs/deploy-checklist.md` — шапка: «для ручной установки; автоматическая — `scripts/install.sh`», и примечание, что скрипт делает те же шаги.
- [ ] **Step 3: Коммит**

```bash
git add README.md docs/deploy-checklist.md
git commit -m "docs: installer quick start"
```

---

### Task 8: release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Написать workflow**

Триггер `push: tags: ['v*']`; шаги: checkout, `actions/setup-node@v4` (node 20), `actions/setup-go@v5` c `go-version-file: go.mod`, `make cross`, `sha256sum bin/flypanel-linux-amd64 scripts/install.sh > sha256sums.txt`, `softprops/action-gh-release@v2` с `files: bin/flypanel-linux-amd64, scripts/install.sh, sha256sums.txt`, `permissions: contents: write`.

**Внутри `sha256sums.txt` имена должны быть без `bin/`** — иначе `sha256sum -c` в скрипте не найдёт файл; делать `cd bin && sha256sum flypanel-linux-amd64` и добавлять `scripts/install.sh` отдельной строкой.

- [ ] **Step 2: Проверить YAML**

Run: `python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml'))"`
Expected: без ошибок.

- [ ] **Step 3: Коммит**

```bash
git add .github/workflows/release.yml
git commit -m "ci: release workflow for tags"
```

---

### Task 9: первый релиз (требует явного согласия на пуш)

**Files:** нет.

- [ ] **Step 1:** `git checkout main && git merge --no-edit dev` — прогнать `make test` и `make web-test`.
- [ ] **Step 2:** `git push origin main`.
- [ ] **Step 3:** `git tag v0.1.0 && git push origin v0.1.0`; дождаться зелёного Actions.
- [ ] **Step 4: Проверить ассеты**

Run: `curl -fsSL https://github.com/dev-by-dev/fly-vpn-panel/releases/latest/download/sha256sums.txt`
Expected: две строки, имена `flypanel-linux-amd64` и `install.sh`.

---

### Task 10: боевые прогоны

- [ ] **Step 1: Ubuntu (fly-hatch, публичный IP, реальный домен)** — `--lang en`; ожидание: HTTPS 200, юзер создан, `/sub/<token>` в 4 форматах, «Мониторинг → Логи» и «Caddy» отдают данные.
- [ ] **Step 2: Повторный запуск** на той же машине: «всё уже стоит», админ цел, домен не дублируется, панель перезапущена.
- [ ] **Step 3: CentOS Stream 9 (VM или VPS)** с `--skip-dns`: пакеты, `firewall-cmd --list-ports` содержит 80/443/8443, `getenforce`, нет свежих AVC, панель на `127.0.0.1:3002`; шаг HTTPS падает с понятным сообщением — это ожидаемо.
- [ ] **Step 4: Негативные** — неверный домен (DNS не сходится) → выход до изменений; занятый порт 80 → выход с именем процесса; не root → ошибка на выбранном языке.
- [ ] **Step 5:** Записать результаты в `docs/superpowers/specs/2026-11-22-installer-design.md` (§ Проверка) и закоммитить.