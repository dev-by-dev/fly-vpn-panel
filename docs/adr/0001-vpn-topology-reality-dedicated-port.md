# ADR-0001: VPN topology — Reality on dedicated port, UI behind own Caddy

**Date**: 2026-11-20
**Status**: accepted (amended 2026-11-20 by ADR-0008: dedicated VPS → native own Caddy)
**Deciders**: product owner (choice A) + agent-operator

## Context

Initial plan: coexist with the existing VPN stack on one server (shared Caddy on 80/443, reload drops all connections). We want full control over the xray config with no intermediary layer. Placement was later changed to a dedicated VPS (ADR-0008) — Caddy became our own, but the topology "Reality separate, UI behind proxy" was kept.

## Decision

Topology A: VPN = VLESS + XHTTP + **Reality on a dedicated port** (default 8443), xray terminates TLS itself — no domain/certificate needed for the VPN. UI and subscriptions — behind our own Caddy (native install, ADR-0008) on the `<PANEL_DOMAIN>` subdomain, block `reverse_proxy 127.0.0.1:3002`. VPN entry — `<ENTRY_IP>`; `sendThrough <EGRESS_IP>` only if a separate egress IP is set (on a dedicated VPS there is one IP — the field is omitted).

## Alternatives Considered

### Alternative 1: B — everything behind one Caddy (VPN + UI on 443)
- **Pros**: one port 443, no need to open a new one.
- **Cons**: h2c layer between Caddy and xray; VPN depends on proxy reload risks; not "full control over the config".
- **Why not**: ties the new VPN's lifecycle to a proxy we do not own — exactly the complexity we are moving away from.

### Alternative 2: C — second Caddy on 8443 + LE DNS-01
- **Pros**: TLS termination by a familiar proxy, isolation from the shared Caddy.
- **Cons**: extra entity (second proxy), certificates via DNS-01 API, renewal chores.
- **Why not**: Reality gives TLS with no certificates at all — a second Caddy is unnecessary.

## Consequences

### Positive
- VPN channel is independent of Caddy: proxy reload does not drop VPN sessions.
- No domain/certificate for the VPN (Reality masquerades as the dest site).
- Caddy serves only the panel: one site, simple edits and rollback.

### Negative
- One more public port (8443) — a firewall rule.
- UI depends on Caddy: reload drops established panel connections.

### Risks
- Caddy reload drops established panel connections → explicit operator approval + runtime config verification (on a dedicated VPS there are no other sites, no quiet window needed).
- Reality dest must be reachable from the client network and support TLS1.3+H2 → default www.microsoft.com:443, configurable.