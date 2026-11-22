# Установщик flypanel для конечного пользователя — дизайн

Дата: 2026-11-22

## Зачем

Сейчас панель ставится по `docs/deploy-checklist.md`: юзер, каталоги, xray с проверкой `.dgst`, `.env`, systemd-юнит, UFW, Caddy, DNS, `admin init`. Это операторский плейбук, не установка для человека, который купил VPS и хочет панель.

Цель: один bash-скрипт на чистом Ubuntu 24.04 или CentOS Stream, который сам всё ставит и спрашивает только то, что нельзя узнать: язык, домен, e-mail для Let's Encrypt, логин админа. Пароль админа вводится в конце в руки `flypanel admin init`.

## Что решено (вариант A)

Один идемпотентный `install.sh`, root на VPS. Варианты B (`curl | bash`) и C (Docker) отклонены: B — тот же скрипт, но с плохой практикой исполнения из сети (всё равно публикуем файл, человек может скачать и прочитать); C — ломает «Мониторинг → Логи» (нет journald), панель спавнит xray внутри контейнера, systemd/UFW мимо.

Решения владельца продукта («везде твой выбор»):

1. **Caddy — официальный статический бинарь с GitHub Releases + свой systemd-юнит.** Никаких сторонних apt-ключей. Цена: обновления Caddy вручную.
2. **DNS строго.** Если A-запись домена ещё не указывает на этот IP — скрипт останавливается до любых изменений и печатает, что прописать у регистратора. Обход — флаг `--skip-dns` (осознанно, для «сначала поставим, потом DNS»).
3. **Охват: Ubuntu 24.04 и CentOS Stream 9/10, amd64.** Debian, RHEL-клоны (Rocky, Alma), arm64, Windows, Docker — не поддерживаем на старте, добавим по факту запросов. CentOS 7/8 не поддерживаем: EOL, скрипт отказывает с понятным сообщением.

## Предусловие: релизы

Скрипт качает панель из GitHub Releases, а их пока нет: `main` на `Initial commit`, `dev` впереди на 46 коммитов.

Порядок:

1. Смержить `dev` в `main`, запушить (сейчас `main` в origin отстаёт).
2. Добавить `.github/workflows/release.yml`: по тегу `v*` — checkout, Node 20, Go из `go.mod` (1.27), `make cross`, `sha256sums.txt`, создать релиз с ассетами `flypanel-linux-amd64`, `install.sh`, `sha256sums.txt`. Репозиторий публичный, Actions бесплатны.
3. Тег `v0.1.0` — первый релиз, на нём и обкатываем скрипт.

Дальнейшие релизы — тег и пуш тега. Версия панели в UI не показывается (вне скоупа).

## Скрипт

Файл `scripts/install.sh`, он же ассет релиза. Публикуется по `https://raw.githubusercontent.com/dev-by-dev/fly-vpn-panel/main/scripts/install.sh` и как `releases/latest/download/install.sh`.

### Каркас

- `set -euo pipefail`, лог в `/var/log/flypanel-install.log` (всё дублируется через `tee`), цвета только при tty.
- `trap` на `ERR`: печатает строку, номер строки, путь к логу. Отката нет — скрипт идемпотентен, повторный запуск догоняет.
- Аргументы: `--lang ru|en|de`, `--domain`, `--email`, `--admin`, `--ip`, `--yes`, `--dry-run`, `--skip-dns`, `--force` (для контейнера и неподдерживаемой ОС). Без флагов — интерактив.
- `--dry-run`: печатает каждое действие без изменений (для проверки на чужой машине).

### Язык

Первый вопрос — `Language / Язык / Sprache [1/2/3]`, дефолт по `$LANG` (`ru*`→ru, `de*`→de, иначе en). Три параллельных массива строк `TXT_RU/TXT_EN/TXT_DE` и индексные константы (`K_WELCOME=0`), функция `t $K_WELCOME`. Никаких gettext/внешних файлов: скрипт должен работать одним файлом.

Переводы покрывают: приветствие, вопросы, предупреждения префлайта, шаги установки, финальную сводку, текст ошибки.

### Префлайт (до любых изменений)

Проверки, каждая — понятное сообщение и выход: root; `x86_64`; systemd (`/run/systemd/system`); поддерживаемая ОС из `/etc/os-release` — `ID=ubuntu` + `VERSION_ID=24.04` → семейство `apt`, либо `ID=centos` + `VERSION_ID` 9/10 → семейство `dnf`; всё остальное (Debian, Rocky, Alma, CentOS 7/8, arm64) — отказ с текстом «поддерживаются Ubuntu 24.04 и CentOS Stream 9/10; для прочих систем ставь вручную по docs/deploy-checklist.md», обход только `--force` (без гарантий); `curl`, `unzip`, `iproute`/`ss` доступны или ставятся пакетным менеджером семейства; диск ≥ 2 ГБ свободно в `/opt`; порты 80, 443, 8443, 3002, 10811 свободны; не в контейнере (`/.dockerenv`, `systemd-detect-virt --container`), иначе `--force`.

Семейство ОС — одна переменная (`FAMILY=apt|dnf`), по ней выбираются пакеты, firewall и SELinux. Общая часть (юзер, каталоги, бинарь, xray, `.env`, systemd, Caddy, проверка HTTPS) от семейства не зависит.

Определение IP: `curl -4 -fsS https://api.ipify.org` (fallback `https://ifconfig.co`), либо `--ip`. Сверка с `ip -4 addr` — если IP не локальный, предупредить (NAT/прокси).

Проверка DNS: `getent hosts <domain>` (fallback DoH `https://dns.google/resolve?name=...&type=A`), сравнить с IP. Не совпало → выход с инструкцией. `--skip-dns` пропускает.

Память: если RAM < 2 ГБ и swap нет — создать `/swapfile` 2 ГБ (шаг из чеклиста), иначе пропустить.

### Вопросы (после префлайта, кроме `--yes` + флаги)

1. Домен панели — обязателен, валидация `^[a-z0-9.-]+\.[a-z]{2,}$`, отказ для `<...>` и `example.com`.
2. E-mail для Let's Encrypt — можно пусто (сертификат выпустится, письма об истечении не придут).
3. Логин админа — дефолт `admin`.
4. Открыть порты в фаерволе? — дефолт да (22/80/443/8443; порт SSH берётся из `sshd -T | grep ^port`, дефолт 22; правило 22 добавляется ДО включения фаервола). На Ubuntu — UFW, на CentOS — firewalld.

Итоговый экран: домен, IP, e-mail, логин, что будет установлено. Подтверждение `[Y/n]`.

### Шаги установки (каждый идемпотентен)

1. Пакеты семейства: Ubuntu — `apt-get update` + `curl ca-certificates unzip ufw iproute2`; CentOS — `dnf install -y curl ca-certificates unzip firewalld policycoreutils-python-utils iproute`, `systemctl enable --now firewalld` (пропуск того, что уже стоит).
2. Юзер `flypanel` (system, home `/opt/flypanel`, nologin) и каталоги `bin data logs backups` — если юзера нет; существующие каталоги не трогаем.
3. Панель: `releases/latest/download/flypanel-linux-amd64` + `sha256sums.txt`, проверка суммы, установка атомарно — скачать в `bin/flypanel.new`, `chown`/`chmod 755`, `mv` поверх `bin/flypanel` (запись поверх работающего бинаря даёт `ETXTBSY`), затем `systemctl restart flypanel`.
4. xray: версия закреплена `XRAY_VERSION=26.11.9`, ассет `Xray-linux-64.zip` + `.zip.dgst`, проверка `SHA2-256=`, распаковка `xray geoip.dat geosite.dat` в `bin/`. Если `bin/xray` уже есть другой версии — предупредить и не перезаписывать без `--force`.
5. `/opt/flypanel/.env` (600, owner flypanel): `LISTEN_ADDR=127.0.0.1:3002`, `DATA_DIR=/opt/flypanel/data`, `XRAY_BIN=/opt/flypanel/bin/xray`, `ENTRY_IP=<IP>`, `EGRESS_IP=`, `PANEL_DOMAIN=<domain>`, `POLL_INTERVAL_SEC=30`, `COOKIE_SECURE=true`. Если файл уже есть — бэкап `.bak-YYYYMMDDHHMM` и перезапись из ответов.
6. `chown -R flypanel:flypanel /opt/flypanel`.
7. systemd-юнит ровно как в чеклисте (включая `PrivateTmp=true`, `ProtectSystem=strict`, `ReadWritePaths=/opt/flypanel`, `SupplementaryGroups=systemd-journal`, `Restart=on-failure`); `daemon-reload`, `enable --now flypanel`.
8. Фаервол (только если пользователь согласился): Ubuntu — `ufw allow <ssh>/tcp`, 80, 443, 8443, затем `ufw --force enable`; CentOS — `firewall-cmd --permanent --add-port=` для тех же портов, `firewall-cmd --reload`. Порт SSH добавляется первым.
9. SELinux (только CentOS, если `getenforce` = Enforcing): после старта панели и Caddy проверить `ausearch -m avc -ts recent`; при отказах на портах — `semanage port -a -t http_port_t -p tcp 8443` и `3002` (если порт уже занят другой меткой — `-m`), затем `systemctl restart caddy flypanel`. Юниты свои, поэтому в норме домен `unconfined_service_t` и запретов нет; шаг закрывает случай, когда политика ужесточена.
10. Caddy: версия закреплена `CADDY_VERSION=2.11.9`, `caddy_2.11.9_linux_amd64.tar.gz` + проверка по `SHA512` из `caddy_2.11.9_checksums.txt`; бинарь в `/usr/local/bin/caddy`, юзер `caddy` (system), каталоги `/etc/caddy` (root) и `/var/lib/caddy` (caddy); systemd-юнит `caddy.service` с `ExecStart=/usr/local/bin/caddy run --environ --config /etc/caddy/Caddyfile`, `Environment=XDG_DATA_HOME=/var/lib/caddy/.local/share`; `/etc/caddy/Caddyfile`:
   ```
   {
       metrics
   }

   <domain> {
       log
       reverse_proxy 127.0.0.1:3002
   }
   ```
   Существующий Caddyfile → бэкап; если в нём уже есть `<domain>` — не дублировать блок. `caddy validate`, `systemctl enable --now caddy` (или `reload`, если уже работает).
11. Ожидание: до 90 с `curl -fsS https://<domain>/` → 200 и в теле `assets/index-`. Не дождались — печатать `journalctl -u caddy -n 30` и выйти с ошибкой (DNS/порт 80/ACME).
12. Админ: `runuser -u flypanel -- env FLYPANEL_ENV=/opt/flypanel/.env /opt/flypanel/bin/flypanel admin status` — код 0 значит админ есть, шаг пропускается. Иначе отдать терминал — `sudo -u flypanel FLYPANEL_ENV=/opt/flypanel/.env /opt/flypanel/bin/flypanel admin init`. При `--yes` — не запускать.
13. Сводка: URL панели, куда логи, `systemctl status`, как обновлять панель (перезапуск скрипта), как откатиться (`docs/deploy-checklist.md` §8), где лежат бэкапы.

### Что скрипт НЕ делает

- Не создаёт DNS-записи, не логинится к регистратору.
- Не трогает провайдерский firewall и порт SSH в sshd.
- Не удаляет данные: `data/panel.db` и `data/backups` не перезаписываются никогда.
- Не обновляет сам себя, панель, xray и Caddy автоматически (повторный запуск скрипта обновляет панель, но не xray/Caddy — их версии закреплены).
- Не настраивает swap, если RAM ≥ 2 ГБ.
- Не ставит второй домен, reverse-proxy на другие сервисы, мониторинг-агенты.

## Границы

- Целевые ОС: Ubuntu 24.04 и CentOS Stream 9/10, amd64. Прочие — отказ с текстом, `--force` без гарантий.
- Caddy слушает 80/443; xray — 8443 + loopback 10811; панель — loopback 3002.
- Интернет нужен: apt/dnf, GitHub Releases (панель, xray, Caddy), Let's Encrypt.

## Проверка

1. `bash -n scripts/install.sh`, `shellcheck` (если есть локально).
2. `--dry-run` в контейнерах `ubuntu:24.04` и `centos:stream9` с `--force` — печатает все шаги с правильным семейством, файлов не создаёт.
3. Прогон на чистом VPS (кандидат — `fly-hatch`, там flypanel неактивен): язык EN → установка → HTTPS 200 → создание юзера → `/sub/<token>` в 4 форматах → «Мониторинг → Логи» и «Caddy» отдают данные.
4. Повторный запуск на той же машине: «всё уже стоит», данные и админ целы, панель перезапущена, домен не дублируется в Caddyfile.
5. Прогон на CentOS Stream 9 (второй чистый VPS): язык RU → установка → HTTPS 200 → `firewall-cmd --list-ports` содержит 80/443/8443 → `getenforce` и отсутствие свежих AVC.
6. Негативные: неверный домен (DNS не сходится) → остановка до изменений; занятый порт 80 → остановка; запуск не от root → понятная ошибка на выбранном языке.
## Уточнения после ревью спеки

1. **Порты и повторный запуск.** Префлайт смотрит владельца через `ss -ltnp`: порт свободен **или** занят нашими же процессами (`flypanel`, `xray`, `caddy`) — проверка пройдена. Чужой процесс на 80/443/8443/3002/10811 → выход с именем процесса. Так повторный запуск на работающей панели не спотыкается о собственные порты.
2. **Проверка «админ уже есть».** В бинарь добавляется `flypanel admin status`: печатает `admin exists` и выходит с кодом 0, если админ есть, иначе «нет админа» и код 1. Никакого `sqlite3` в зависимостях, WAL не читаем снаружи. Это единственное изменение Go-кода в фиче; на него — тест в `cmd/flypanel`.
3. **Флаги.** `--dry-run` выполняет только читающие проверки и печатает шаги, ничего не меняя; в dry-run провал DNS, занятый порт, контейнер и неподдерживаемая ОС — предупреждения, а не выход. `--force` снимает блокировки ОС/контейнера/DNS/портов (для отладки и чужих систем), но не отключает проверку SSH-порта. Каждая проверка — отдельная функция, чтобы список исключений был явным.
4. **SSH-порт и фаервол.** Порт берётся из `sshd -T`, при неудаче — из `ss -ltnp` по процессу `sshd`. Если оба способа не дали порт — **скрипт не включает фаервол** и выходит с сообщением, как открыть порты вручную (флаг `--skip-firewall` позволяет продолжить без настройки). Заблокировать себе SSH скрипт не может.
5. **Смена домена при повторном запуске.** Блок сайта в Caddyfile оборачивается маркерами `# flypanel-managed begin` / `# flypanel-managed end`. Повторный запуск заменяет содержимое маркеров новым доменом, старый блок не остаётся жить. Если маркеров нет, а файл непустой и в нём нет блока для запрошенного домена — выход: чужой конфиг не трогаем.
6. **Чужой Caddyfile.** Глобальный блок `{ metrics }` добавляется только если глобального блока нет; если есть чужой глобальный блок без `metrics` — выход с текстом (ручное слияние), не правим. Существующий сайт на 80/443, не наш — выход: иначе ACME и проверка HTTPS упадут на чужом сайте.
7. **Мелочи исполнения.** Админ-шаг через `runuser` (не `sudo`, в минимальном CentOS его может не быть). Провал `admin init` — предупреждение и код выхода 1 в конце, панель уже установлена и работает; повторный запуск даст ввести пароль снова.
