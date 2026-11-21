# Inbound VLESS+TCP+Reality и подписки под любой клиент

## Задача

Профиль панели — VLESS+XHTTP+Reality. На планшете (Huawei, Android 13) его не
принимает ни один клиент:

- Incy 3.9.9 при импорте выбрасывает `type=xhttp` и сохраняет VLESS·TCP без TLS;
- Hiddify 5.5.5-dev при подключении не отправляет ни одного пакета;
- v2rayNG 2.3.15 разбирает профиль верно, но его ядро гоняет исходящий трафик в
  собственный tun0 (на сервер не приходит ни байта);
- Happ падает на сборке REALITY: `empty "serverNames"`.

Сервер и ссылка при этом исправны: настоящий Xray 26.11.9 с тем же конфигом
успешно ходит через туннель (api.ipify.org → 217.177.10.159).

## Решение

1. Добавить второй inbound VLESS+TCP+Reality (порт 8444) — транспорт, который
   понимают все перечисленные клиенты.
2. Научить рендереры подписок сети отличной от XHTTP:
   - `links.go`: `linkParams.Net` из `streamSettings.network` (по умолчанию
     `xhttp` — обратная совместимость); `xhttpSettings` обязателен только для
     xhttp; ссылка получает `type=<net>`, а `path`/`mode` — только для xhttp.
   - `yaml.go`: `network: <net>`, `xhttp-opts` только для xhttp.
   - `json.go` + `default_json.json`: плейсхолдер `{{NETWORK}}`.

## Проверка

- `go test ./internal/sub/` — новый `TestRenderBase64TCP`: две строки в base64,
  у xhttp сохранены `path`/`mode`, у tcp — `type=tcp`, `security=reality`,
  `encryption=none`, `pbk/sid/sni`, `fp=chrome`, без `path`/`mode`.
- `go test ./...` — зелёный.
- После деплоя: подписка содержит две ссылки, TCP-инбаунд слушает 8444,
  подключение с планшета проходит.

## Вне рамок

- JSON-шаблон для TCP-инбаунда: остаётся xhttp-шаблон (клиенты для TCP берут
  base64/mihomo).