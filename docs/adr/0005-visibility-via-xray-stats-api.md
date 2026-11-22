# ADR-0005: Traffic/online visibility via xray gRPC stats API

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: product owner (hybrid accepted) + agent-operator (implementation via API)

## Context

The main pain of the existing panel is that it is uninformative: it is unclear who is connected and how much traffic they consume. xray stats are keyed by user (uuid), not by device; behind NAT one IP covers a whole router. We need visibility without unreliable heuristics.

## Decision

Hybrid **purely via the xray gRPC StatsService**: `QueryStats` per-user uplink/downlink → deltas in SQLite → day/week/month aggregates; `GetAllOnlineUsers` + `GetStatsOnlineIpList` → online IPs + last-seen per user. Polling every 30 s. "Device" = distinct source IP (NAT caveat in the UI). Parsing the access log is NOT needed.

## Alternatives Considered

### Alternative 1: per-device traffic
- **Pros**: an accurate per-device picture.
- **Cons**: xray does not attribute traffic per device (uuid only); unreliable behind NAT.
- **Why not**: it cannot be done reliably — we do not promise it.

### Alternative 2: parsing the xray access log
- **Pros**: works even if the stats API does not return online IPs.
- **Cons**: more complex (parser, log rotation), duplicates what the API already provides.
- **Why not**: research on 20.11.2026 showed GetStatsOnlineIpList/GetAllOnlineUsers in xray — the API covers the task. Kept as a fallback only if the method semantics are not confirmed.

## Consequences

### Positive
- Clean implementation on the API only, no file parsers.
- Live list of online devices + per-user traffic graphs.

### Negative
- "Device" = distinct IP: one router = one device (NAT caveat).
- The semantics of the online methods must be checked against the pinned xray release during implementation.

### Risks
- The API does not return the IP in the required form → fall back to access-log (only then).
- Counters reset on xray restart → deltas are recorded every 30 s, loss ≤ the interval.
- policy statsUserOnline=false → online IPs are not collected; forced to true in the config.