# ADR-0003: xray as panel-owned child process

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: agent-operator (proposed) + product owner (accepted into v1 scope)

## Context

We want full control over the xray config and lifecycle with no intermediary layer (the node agent of the existing panel is exactly the complexity we are moving away from). Precedent 3x-ui: the panel itself owns the xray process.

## Decision

The panel starts xray as a **child process** and owns its lifecycle entirely. The config is generated from the DB (DB = source of truth). Structural changes: validate `xray run -test` → write → restart the child (~1–2 s). Users/quotas/limits/expiry: hot, via gRPC `HandlerService.AlterInbound` without restart. xray crash → auto-restart with backoff.

## Alternatives Considered

### Alternative 1: separate systemd unit for xray
- **Pros**: independent restart, familiar ops.
- **Cons**: two lifecycle owners, config/process desync, the panel cannot apply config atomically.
- **Why not**: desync is the source of those very "invisible" breakages in the existing panel.

### Alternative 2: node agent (an intermediary layer like in Remnawave)
- **Pros**: multi-node out of the box.
- **Cons**: extra entity, its own API/secrets, config generator outside our control.
- **Why not**: multi-node is a v1 non-goal; the intermediary layer is the main pain we are moving away from.

## Consequences

### Positive
- One owner: atomic config application, validation before restart.
- Hot user operations without dropping connections.
- The resulting config is always visible in the UI.

### Negative
- Panel crash kills xray (child).
- Structural edits = 1–2 s downtime.

### Risks
- Panel crash → systemd Restart=always for flypanel + auto-restart of xray with backoff.
- Invalid config → `xray run -test` before writing; on failure the old config is untouched.
- Stats counters reset on xray restart → frequent polling (30 s), deltas in SQLite.