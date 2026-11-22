# ADR-0006: Traffic quota with per-user reset period and auto re-enable

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: product owner (choice in builder-round 20.11.2026) + agent-operator

## Context

Users come from Remnawave, where the traffic limit usually has a reset period. A builder-check of the design doc revealed: quota semantics (lifetime vs resettable, what counts, restoration after disabling) change the DB schema and the enforcement logic — guessing is not allowed.

## Decision

Per-user quota: `quota_bytes` (**up+down combined**, 0 = unlimited) + `quota_period` (none | day | week | month) + `quota_reset_at`. The periodic counter = the sum of deltas from traffic_samples since the period start. Exceeding → hot disable (RemoveUser + disabled_reason=quota). Start of a new period → the counter resets + **auto re-enable**. The admin can reset/raise the limit manually → auto re-enable. none = lifetime limit (restored manually only).

## Alternatives Considered

### Alternative 1: lifetime quota + manual reset
- **Pros**: simplest of all — one field, one enforcement branch.
- **Cons**: does not match the "limit with reset" expectation from Remnawave; the admin resets manually every period.
- **Why not**: the user explicitly chose a period + auto-reset.

## Consequences

### Positive
- Matches the Remnawave habit, honest repeating limits.
- Auto-restore without manual admin action.

### Negative
- Extra fields (period, reset_at, disabled_reason) + rollover logic in the poller.
- The counter is derived from samples (or a column) — a schema detail left to the builder.

### Risks
- Missed rollover while the panel is down → check `now ≥ quota_reset_at` on every poll and at startup.
- Double disable/enable (race between the poller and manual actions) → single writer + explicit disabled_reason.