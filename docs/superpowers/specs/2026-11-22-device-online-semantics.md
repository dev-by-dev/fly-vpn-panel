# Online semantics for devices (HWID)

**Date:** 2026-11-22
**Status:** implemented (commit `2821881`), deployed
**Related files:** `internal/stats/poller.go`, `internal/store/devices.go`, `internal/api/handlers.go`, `web/src/routes/Users.svelte`

## What happened

The “HWID” tab showed four green dots with only one client actually connected.

Production data (a copy of `panel.db`, read-only):

```
id  username     ip              hwid              ua                             last_seen
 5  clash-verge  198.51.100.47   C6BF0F6F-…B2E5    Happ/4.8.1/macOS              2026-11-22T10:10:32Z
 9  clash-verge  198.51.100.47   b91035b1c0dfd54b  Happ/4.11.2/Android           2026-11-22T10:10:32Z
10  clash-verge  198.51.100.47   b45d2f0b8bbf4369  Happ/4.11.2/Android           2026-11-22T10:10:32Z
11  clash-verge  198.51.100.47   (empty)           HiddifyNext/5.9.15 (android)  2026-11-22T10:10:32Z
```

One user, one IP, `last_seen` matching to the second — so the poller stamped those rows, not the clients.

## Cause

1. `stats.Poller` called `DeviceUpsert(name, ip, meta{}, now)` for every online IP, and with empty meta that runs
   `UPDATE devices SET last_seen=? WHERE username=? AND ip=?` — stamping **every** row of that IP (different apps, stale HWIDs after a reinstall, empty HWID).
2. Then the poller called `DeviceTouchUser(name, now)` — `UPDATE devices SET last_seen=? WHERE username=?` — reviving literally **all** of the user's devices, including rows from other IPs.
3. Online was computed in the API as `last_seen >= now − 2·interval`, so every stamped row turned green.

The main defect is (2): a device that is not on the network looked online forever as long as the user was connected from anywhere.

## Constraint that cannot be worked around

xray only knows “user is online from IP X”. xray stats carry no HWID. Behind NAT several devices share one public IP, so the panel cannot tell which device is connected — only which address the user is currently coming from.

## Accepted semantics (option A)

- Online is defined **per (user, IP) pair**, not per device.
- In each pair exactly one row counts as online — the freshest one; on equal `last_seen`, the first in the output.
- The remaining rows of the same pair are returned as `same_ip: true` and drawn as a grey dot labelled “same IP”.
- Rows with an IP that is not online right now age out on their own and turn red.
- `DeviceTouchUser` is removed; the poller only touches devices with a genuinely online IP.

The `GET /api/devices` API returns `online` and `same_ip` per row; the per-user `online_count` and `summary.online_count` are unchanged (they count users, not devices).

## Non-goals

- Exact device identification behind NAT — impossible without HWID in xray stats.
- Splitting rows of one IP by User-Agent (two different clients from one address are indistinguishable).
- Connection history and “was online N minutes ago” beyond the `last_seen` field.
- Changing the polling frequency or the online window (`2 × interval`).

## Verification

- `internal/stats/poller_test.go` → `TestOnlineDevicesIgnoreOtherIPs`: a device whose IP is not online is not updated (RED before the fix: `last_seen = 10:27, want untouched 08:27`).
- `internal/api/devices_test.go` → `TestDevicesOneOnlinePerIP`: two rows on one IP + one on another → `online == 1`, `same_ip == 1` (RED before the fix: `online rows = 2, want 1`).
- `web/src/routes/Users.test.ts` → “devices behind one IP: one green dot and a ‘same IP’ label” (RED before the fix: no “same IP” text).
- The existing `TestOnlineDevices` (both IPs online) keeps passing.
- Full run: `go test ./...` — ok; `npx vitest run` — 34/34; `npm run check` — 0 errors, 12 pre-existing warnings.

## Residual behavior

`last_seen` for rows of one IP is still updated by the poller (online is measured by IP). Only the display changes: one green dot per pair. If xray ever starts returning HWID in `GetStats`, the “user + IP” pair can be replaced with “user + HWID” — and the NAT non-goal goes away.
