# Деинсталлятор flypanel — дизайн

## Зачем

После установки скриптом `scripts/install.sh` на сервере остаются юзер, бинари, юниты, правки Caddyfile, правила фаервола и пакеты. Нужен `scripts/uninstall.sh`, который возвращает машину в состояние «как до установки»: одна команда, те же три языка, `--dry-run`, идемпотентность.

## Что именно оставил установщик

Сверено с `scripts/install.sh`:

1. юзер `flypanel` и `/opt/flypanel/{bin,data,logs,backups}`;
2. `/opt/flypanel/bin/{flypanel,xray,geoip.dat,geosite.dat}`, `/opt/flypanel/.env` (+ `.bak-*`);
3. `/etc/systemd/system/flypanel.service` (enabled, запущен, xray — его дочерний процесс);
4. Caddy: `/usr/local/bin/caddy`, юзер `caddy`, `/var/lib/caddy`, `/etc/systemd/system/caddy.service`, блок `# flypanel-managed begin|end` в `/etc/caddy/Caddyfile`, возможно добавленный глобальный блок `{ metrics }`;
5. правила ufw/firewall-cmd: SSH-порт, 80, 443, 8443; `ufw --force enable`;
6. CentOS: `semanage port` для 8443 и 3002;
7. пакеты (`curl ca-certificates unzip ufw iproute2` / `curl ca-certificates unzip firewalld policycoreutils-python-utils iproute`);
8. `/var/log/flypanel-install.log`.

## Решение по данным (выбор пользователя)

`/opt/flypanel/data` удаляется целиком, включая `panel.db` — без архива и без бэкапа. Защита от случайного запуска: интерактивно требуется ввести слово `delete`; `--yes` пропускает вопрос.

## Что удаляется и что нет

| Объект | Действие |
|---|---|
| юнит `flypanel`, `caddy` (наш) | stop, disable, `rm`, `daemon-reload` |
| `/opt/flypanel` целиком | `rm -rf` (данные тоже) |
| юзеры `flypanel`, `caddy` | `userdel` (без `-r`: домашние каталоги уже снесены) |
| `/usr/local/bin/caddy` | удалить |
| наш блок в Caddyfile | вырезать; если в файле не осталось ничего, кроме глобального `{ metrics }`, — удалить файл и каталог `/etc/caddy` |
| `/var/lib/caddy` | удалить |
| правила фаервола 80/443/8443 | удалить (`ufw delete allow`, `firewall-cmd --permanent --remove-port`) |
| правило SSH-порта | **не трогаем** (риск лок-аута) |
| `ufw`/`firewalld` | **не выключаем** (не знаем, был ли включён до установки; молча снять защиту хуже, чем оставить) |
| пакеты | удаляем только те, что поставил установщик (список в `$PANEL_DIR/installed-packages`); предустановленные не трогаем, `--keep-packages` оставляет все |
| `semanage port` 8443/3002 | удалить (только CentOS, только если порт помечен `http_port_t`) |
| `/var/log/flypanel-install.log` | удалить вместе с логом деинсталляции |

Чужая конфигурация Caddy (сайты не от нас, чужой глобальный блок) не трогается: если маркеров `flypanel-managed` нет, файл остаётся как есть, скрипт об этом говорит.

## Флаги

`--lang ru|en|de`, `--yes`, `--dry-run`, `--skip-firewall`, `--force` (продолжать при непонятном состоянии), `-h`.
`--dry-run` только печатает шаги, ничего не меняя; любые «страшные» состояния там — предупреждения, а не выход.

## Шаги скрипта

1. root-проверка (до открытия лога), выбор языка, баннер + предупреждение, что данные исчезнут.
2. Подтверждение: слово `delete` (или `--yes`).
3. `systemctl stop flypanel` → xray умирает вместе с cgroup.
4. `systemctl stop caddy`, `systemctl disable flypanel caddy`, `rm` юнитов, `daemon-reload`.
5. Caddyfile: вырезать managed-блок awk-ом; пустой остаток → удалить файл и `/etc/caddy`.
6. `/opt/flypanel` → `rm -rf`; `/usr/local/bin/caddy`, `/var/lib/caddy` → `rm -rf`.
7. `userdel flypanel`, `userdel caddy` (если существуют и не заняты процессами).
8. Фаервол: удалить 80/443/8443, SSH-порт оставить; `--skip-firewall` пропускает шаг.
9. CentOS: `semanage port -d` для 8443 и 3002, если помечены нашим типом.
10. Логи: удалить `/var/log/flypanel-install.log`, затем сам лог деинсталляции.
11. Итог: что удалено, что оставлено намеренно (SSH-правило и то, что было до установки).

## Не-цели

- не откатывать DNS/ACME-сертификаты;
- не удалять пакеты, которых не было до установки, и не выключать чужой фаервол;
- не работать на чужих ОС: Ubuntu 24.04 / CentOS Stream 9-10, amd64 (как установщик);
- не «частичное» удаление по флагам (кроме `--skip-firewall`).

## Проверка

1. `bash -n`, shellcheck.
2. Юнит-тесты в `scripts/uninstall_test.sh` (тот же приём `INSTALL_LIB_ONLY`-подобного sourcing-гарда): вырезание managed-блока (есть/нет/чужой сайт), решение «удалять ли Caddyfile» (только `{ metrics }` → удалить), `--dry-run` ничего не создаёт и не удаляет.
3. Живой прогон на VM `home-fly` (Ubuntu 24.04, установлено v0.1.2): после деинсталляции — нет юзера, нет `/opt/flypanel`, нет юнитов, `ss -ltn` без 3002/10811/8443, Caddyfile отсутствует, ufw активен без наших портов; повторный запуск — «нечего удалять», exit 0.
4. Затем повторная установка тем же `install.sh` на очищенной машине — должна пройти с нуля (проверка, что деинсталлятор ничего не сломал системе).
5. Публикация: `scripts/uninstall.sh` добавляется в релизные ассеты CI, README получает раздел «Uninstall».
## Результаты проверки (VM home-fly, Ubuntu 24.04)

- `scripts/uninstall_test.sh`: 14 проверок, 0 падений (bash 5 на VM; на macOS bash 3.2 ассоциативных массивов не умеет).
- Живой прогон на установке v0.1.2: юзер `flypanel` удалён, `/opt/flypanel` отсутствует, оба юнита сняты, порты 3002/10811/8443 свободны, `https://devbydev.crazedns.ru` не отвечает, `ufw` остался активен, из правил осталось только `22/tcp`, сервис `flypanel` и `caddy` — inactive.
- Повторный прогон: «Удалять нечего: панель не установлена», exit 0. Запуск без root: «Нужен root», exit 1, до открытия лога.
- Найдено и исправлено на живом прогоне: бекапы `/etc/caddy/Caddyfile.bak-*` (их делает установщик) не давали удалить каталог. Теперь при удалении нашего Caddyfile они удаляются вместе с ним (покрыто тестом 5).
- Обратная установка тем же опубликованным `install.sh` (v0.1.2, sha256 `323537c5…`) на очищенную машину: все 13 шагов «готово», HTTPS 200, `flypanel` и `caddy` active, ufw снова 22/80/443/8443, сертификат Let's Encrypt выдан заново, admin создан, `flypanel version` → `v0.1.2`.
