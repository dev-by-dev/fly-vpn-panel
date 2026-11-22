# ADR-0007: Public-repo anonymization policy

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: product owner (directive 20.11.2026) + agent-operator

## Context

The repository is public and will be pushed to public GitHub. The draft design doc contained real IPs, domains, server aliases, user names and paths to secrets. Publishing such data exposes the operator's infrastructure.

## Decision

All repo artifacts (docs, code, comments, fixtures, tests, git history) are **anonymized**: no real addresses, names, server/provider aliases, paths to secrets, or details of the client network. Placeholders: `<ENTRY_IP>`, `<PANEL_DOMAIN>`, `<EGRESS_IP>` (optional). Real values live only on the server (`/opt/flypanel/.env`, chmod 600, gitignored) and in the operator's private runbook outside the repo.

## Alternatives Considered

### Alternative 1: private repo
- **Pros**: real values can be written.
- **Cons**: does not match the owner's intent (public project).
- **Why not**: the user explicitly wants public.

### Alternative 2: separate private repo for sensitive docs
- **Pros**: public code + private details.
- **Cons**: two sources of truth, drift, harder to maintain.
- **Why not**: one public repo + server config + private runbook is simpler and drift-free.

## Consequences

### Positive
- Safe to publish; the builder works from placeholders.
- Secrets and topology do not leak into git history.

### Negative
- The deploy runbook with real values lives outside the repo (private journal).
- Docs cannot be copy-paste-run without the operator's values.

### Risks
- A draft commit with real data is already in the local history → rewrite BEFORE the first push (amend the unpushed commit).
- Accidental leak in future code/tests → anonymization check before every push (design doc §9), .gitignore on .env/data/*.db.