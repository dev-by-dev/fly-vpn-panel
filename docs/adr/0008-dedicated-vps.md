# ADR-0008: Dedicated VPS instead of coexistence

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: product owner (initiative) + agent-operator

## Context

The original design assumed coexistence: the new panel on the same server as the live production VPN stack (shared Caddy, docker network, occupied ports). Risks: a Caddy reload drops connections for all sites (including the operator's SSH through the tunnel), the bind-inode trap, shared ports, firewall edits next to production. The user proposed standing up a separate VPS. Reconnaissance confirmed the minimal configuration is sufficient: 1 vCPU / 1 GB RAM / 10 GB disk, Ubuntu 24.04, one public IP, ports 80/443/8443/3002/10811 free, no swap.

## Decision

The new panel is deployed on a **dedicated VPS** (1 vCPU / 1 GB / 10 GB, one IP, entry=egress). The old server is not touched at all during deploy. Caddy — native install from scratch, the panel binds `127.0.0.1:3002`, swapfile 2 GB, Docker not needed. `sendThrough` is omitted (one IP); the `<EGRESS_IP>` field is kept as headroom for a future "entry≠egress".

## Alternatives Considered

### Alternative 1: Coexist on the old server (original plan)
- **Pros**: no new server needed, DNS already points there.
- **Cons**: risk to the production stack (reload drops all connections, including the operator's SSH), quiet windows, docker-gateway bind, shared ports/firewall.
- **Why not**: the user chose isolation; the cost of a cheap VPS is below the risk of breaking a working channel.

## Consequences

### Positive
- Isolation by construction: a deploy physically cannot break the existing stack.
- Own Caddy: no inode traps and no quiet windows, simple rollback.
- Simpler topology: loopback bind, no docker network, no regression of the old stack.

### Negative
- An additional server (cost, administration).
- DNS: the A record for `<PANEL_DOMAIN>` must be pointed at the new IP during deploy.
- Migrating users from the old panel — a separate task later.

### Risks
- 1 GB RAM: a 2 GB swap is mandatory; build on the operator's machine (cross-compile), not on the VPS.
- The provider agent occupies 8080/8189 — do not touch.
- One IP: for a future "entry≠egress", add a second IP and set `<EGRESS_IP>` (the generator already supports it).