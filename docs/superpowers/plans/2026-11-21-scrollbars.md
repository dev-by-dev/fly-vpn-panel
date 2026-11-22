# Visible scrollbars

## Task

On macOS overlay scrollbars are not visible until you start scrolling the page,
so the panel looked like "the page doesn't move". Constantly visible scrollbars
are needed.

## Solution

In `web/src/app.css`, `@layer base`: styling for `::-webkit-scrollbar`
(10 px, dark thumb, transparent track). The `scrollbar-width` property is
deliberately not set: in Chrome it overrides `::-webkit-scrollbar` and the
browser goes back to overlay scrollbars.

## Verification

- Chromium, 1366 px: `innerWidth - documentElement.clientWidth = 10` — the bar
  takes up space and is visible in the screenshot.

## Out of scope

- Firefox: `scrollbar-width/color` not added so as not to break Chrome.