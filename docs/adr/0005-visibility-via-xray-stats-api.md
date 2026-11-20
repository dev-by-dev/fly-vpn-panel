# ADR-0005: Traffic/online visibility via xray gRPC stats API

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: владелец продукта (гибрид принят) + агент-оператор (реализация через API)

## Context

Главная боль действующей панели — неинформативна: непонятно, кто подключён и сколько трафика потребляет. xray stats keyed по юзеру (uuid), не по устройству; за NAT один IP на весь роутер. Нужна видимость без ненадёжных эвристик.

## Decision

Гибрид **чисто через xray gRPC StatsService**: `QueryStats` per-user uplink/downlink → дельты в SQLite → агрегаты день/неделя/месяц; `GetAllOnlineUsers` + `GetStatsOnlineIpList` → онлайн-IP + last-seen по юзеру. Поллинг 30 с. «Устройство» = distinct source IP (NAT-оговорка в UI). Парсинг access log НЕ нужен.

## Alternatives Considered

### Alternative 1: per-device трафик
- **Pros**: точная картина по устройствам.
- **Cons**: xray не атрибутирует трафик по устройствам (только uuid); за NAT ненадёжно.
- **Why not**: технически не сделать надёжно — не обещаем.

### Alternative 2: парсинг access log xray
- **Pros**: работает даже если stats API не даст онлайн-IP.
- **Cons**: сложнее (парсер, ротация логов), дублирует то, что API уже отдаёт.
- **Why not**: ресёрч 20.11.2026 показал GetStatsOnlineIpList/GetAllOnlineUsers в xray — API покрывает задачу. Оставлен как fallback только если семантика методов не подтвердится.

## Consequences

### Positive
- Чистая реализация только на API, без файловых парсеров.
- Живой список онлайн-устройств + графики трафика по юзерам.

### Negative
- «Устройство» = distinct IP: один роутер = одно устройство (NAT-оговорка).
- Семантику онлайн-методов нужно проверить на пиннутом релизе xray при имплементации.

### Risks
- API не отдаёт IP в нужном виде → fallback на access-log (только тогда).
- Счётчики сбрасываются при рестарте xray → дельты фиксируются каждые 30 с, потеря ≤ интервала.
- policy statsUserOnline=false → онлайн-IP не собираются; в конфиге принудительно true.
