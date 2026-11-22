# ADR-0004: Svelte 5 SPA embedded in Go binary

**Date**: 2026-11-20
**Status**: accepted
**Deciders**: agent-operator (choice delegated by the product owner)

## Context

The frontend must be "lightweight, concise, beautiful, modern, possibly hyped" (user requirement, choice delegated). Deploy must stay one file. Dark theme in the spirit of Remnawave. No SSR needed — the panel is behind auth, single admin.

## Decision

**Svelte 5 (runes) + Vite + Tailwind CSS v4 + shadcn-svelte**; the SPA is built and embedded into the Go binary via `embed.FS`. Verified compatible version set: Svelte 5.59, Vite 8.3.8, vite-plugin-svelte 7.3.1, Tailwind 4.3.5, shadcn-svelte 1.8.1 (≥1.0 = Svelte 5 + TW4 only). Node ≥20.19/22.12 on the build machine only.

## Alternatives Considered

### Alternative 1: React / Next.js
- **Pros**: largest ecosystem, more ready-made components.
- **Cons**: heavier bundle and runtime; Next SSR is unnecessary (one admin behind auth).
- **Why not**: extra weight with no benefit for this task.

### Alternative 2: server templates (html/template + htmx)
- **Pros**: zero JS build, even lighter.
- **Cons**: not "beautiful/modern" per the requirement; interactive charts/editors are harder to build.
- **Why not**: does not match the explicit appearance requirement.

## Consequences

### Positive
- Light bundle, no node runtime on the server, deploy = one file.
- Modern DX (runes), ready-made shadcn-svelte components, dark theme out of the box.

### Negative
- Svelte ecosystem is smaller than React (niche libs take longer to find).
- Versions must be pinned (the set is verified, but npm drifts).

### Risks
- Version incompatibility during install → pin the verified set at implementation start.
- Heavy chart libs will bloat the bundle → the builder picks a light lib, no dashboard frameworks.