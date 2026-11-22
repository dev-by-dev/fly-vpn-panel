# Routing moves into the Config tab

## Why

The Routing tab is the only place where xray's server-side `routing` section is
edited (block list + `domainStrategy`). The generated config only has the
`direct` and `block` outbounds, so there is no full “proxy to the outside” there and
the screen is narrow in meaning. As a top-level tab next to seven others it takes
space but is rarely needed.

## What we do

1. Remove `Routing` from the top nav and from the route table in `App.svelte`.
2. Delete `Routing.svelte`; move its editor (textarea + “Check” + “Apply”,
   the same warning about instant application and rollback) to the end of
   `Config.svelte` as a separate “Routing (advanced JSON)” section.
3. Point old `#/routing` bookmarks at Config: keep `'/routing': Config` in the
   route table — one line, the page does not come up empty.
4. The backend is untouched: `/api/config/routing` GET/PUT stay as they are.

## Boundaries

- We do not add rule presets or schema validation — migration only.
- We do not change the look of the config generator or the default `{"domainStrategy":"AsIs","rules":[]}`.
- Config loads both endpoints in parallel, section errors are independent: if
  routing fails to load, the main config is still shown.

## Verification

- `web`: a new `Config.test.ts` — routing loads, the PUT goes out with the text from
  the textarea, 422 shows the rollback message.
- `go test ./...` and `make web-test` green; `make build` builds.
