# ADR-0002: Go + SQLite single-binary backend

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: product owner (Go — requirement, SQLite — confirmation) + agent-operator

## Context

The server has limited resources (~2 vCPU / ~4 GB RAM) and is already occupied by the existing stack. We want a minimum of entities and maintenance: deploy = one file. Precedent 3x-ui = Go + SQLite + xray. The backend language was set by the user (Go); the DB was delegated ("lightweight and fast").

## Decision

One static Go binary `flypanel` (HTTP via stdlib net/http, SPA through embed.FS) + **SQLite** (modernc.org/sqlite, pure Go; WAL + `_busy_timeout` + `_synchronous=FULL`) + the xray binary next to it. DB backup = a copy of the file.

## Alternatives Considered

### Alternative 1: PostgreSQL
- **Pros**: multi-writer, replication, familiar ecosystem.
- **Cons**: +container, +backups, +RAM, +maintenance; on a single server with a single writer the gain is zero.
- **Why not**: complexity with no benefit until thousands of users/multi-node (v1 non-goal). The existing panel already drags postgres along — we do not want to repeat that.

### Alternative 2: Node / Rust / Python backend
- **Pros**: Node — one language with the frontend; Rust — performance; Python — development speed.
- **Cons**: Node — heavier runtime; Rust — slower development; Python — runtime + dependencies. None has a native advantage for the xray gRPC API.
- **Why not**: Go = static binary, tens of MB of RAM, native gRPC client, user requirement.

## Consequences

### Positive
- Deploy = one file (+ xray), RAM in the tens of MB.
- Backup/restore = a copy of the file, zero DB maintenance.
- Native gRPC for xray StatsService/HandlerService.

### Negative
- SQLite: single writer (enough for one panel).
- No shared DB for multi-node (v1 non-goal).
- pure-Go driver is slower than C bindings (irrelevant at this load).

### Risks
- DB file corruption → daily backup (7 copies) + WAL + `_synchronous=FULL`.
- Concurrent writes from several goroutines → single writer + busy_timeout.