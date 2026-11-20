# Architecture Decision Records

Формат: Michael Nygard (адаптированный). Статусы: proposed → accepted → deprecated / superseded.
Все ADR обезличены (публичный репо): плейсхолдеры `<ENTRY_IP>`, `<EGRESS_IP>` (опциональный), `<PANEL_DOMAIN>` — см. дизайн-док §3.

| ADR | Title | Status | Date |
|-----|-------|--------|------|
| [0001](0001-vpn-topology-reality-dedicated-port.md) | VPN topology: Reality on dedicated port, UI behind own Caddy | accepted (amended by 0008) | 2026-11-20 |
| [0002](0002-go-sqlite-single-binary.md) | Go + SQLite single-binary backend | accepted | 2026-11-20 |
| [0003](0003-xray-child-process.md) | xray as panel-owned child process | accepted | 2026-11-20 |
| [0004](0004-svelte-embedded-spa.md) | Svelte 5 SPA embedded in Go binary | accepted | 2026-11-20 |
| [0005](0005-visibility-via-xray-stats-api.md) | Traffic/online visibility via xray gRPC stats API | accepted | 2026-11-20 |
| [0006](0006-quota-reset-period.md) | Traffic quota with per-user reset period and auto re-enable | accepted | 2026-11-20 |
| [0007](0007-public-repo-anonymization.md) | Public-repo anonymization policy | accepted | 2026-11-20 |
| [0008](0008-dedicated-vps.md) | Dedicated VPS instead of coexistence | accepted | 2026-11-20 |
