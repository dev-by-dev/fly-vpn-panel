# Logged-in user on /login saw the login card inside the working layout

## Symptom

With `#/login` open and an active session, the panel rendered the authenticated
layout (header with tabs) and the Login card inside `<main>` — top left.
It looked like "the layout fell apart", "not centered", "not full width".

## Cause

The `/login` route was present in the route map regardless of session
state. The header and `main` render based on `authState.loggedIn`, so Login
ended up inside the working frame.

## Solution

`web/src/App.svelte`:

- import `Router, { router, replace }`;
- effect: if `authState.loggedIn && router.location === '/login'` —
  `replace('/')` (without writing to history);
- `nav` got `overflow-y-hidden` — together with `overflow-x-auto` and the global
  10px webkit scrollbars, a vertical bar appeared in the header.

A logged-out user on `/login` still sees a separate centered screen.

## Verification

- Logged in, `#/login` open → hash `#/`, h1 «Главная», no Login card.
- Logged out, `#/login` → centered card without the header.
- `npm run check` 0 errors, `npx vitest run` 22 tests, `go test ./...` ok.

## Out of scope

Redirect from other protected routes to `/login` on an expired session —
behavior unchanged.