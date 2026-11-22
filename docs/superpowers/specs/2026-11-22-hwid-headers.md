# Spec: HWID and device data from client headers

## Context

Right now the panel only knows the username and the source IP address (that is all
xray returns). In “HWID / devices” the platform, version, model and User-Agent
are shown as “—”. In Remnawave those fields are filled because the client sends
them as headers when it requests the subscription.

Happ, INCY, v2rayNG and mihomo-compatible clients send the following on a
subscription fetch:

| Header | Value |
|---|---|
| `X-HWID` | device identifier from the client |
| `X-Device-OS` | `Windows` / `iOS` / `Android` / `macOS` / `Linux` |
| `X-Device-Model` | model (`iPhone 15 Pro`, `SM-S938B`, `DESKTOP-…`) |
| `X-Ver-OS` | OS version (`11_10.0.26100`, `17.2`) |
| `User-Agent` | client and version (`Happ/4.8.1/Windows/…`) |

Verified on our clients: the inbound panel already sees these requests but does
not store the headers.

## Goal

Show the real HWID, platform, OS version, model and User-Agent in “HWID / devices”
instead of “—”, and the device count in the users table (already there, the Σ
column).

## Solution

### Storage

Migration `0002_device_headers.sql` — add nullable columns to `devices`:

```sql
ALTER TABLE devices ADD COLUMN hwid TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN platform TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN os_version TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN model TEXT NOT NULL DEFAULT '';
ALTER TABLE devices ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
```

`store.Device` gets the matching fields; `DeviceUpsert` grows to
`DeviceUpsert(username, ip string, meta DeviceMeta, now time.Time)`, where empty
fields do not overwrite previously stored values (a client may not send all of them).

### Collection

In `sub.Handlers.Sub` (before serving the subscription):

1. Take the IP from `X-Forwarded-For` **only** if the request came from loopback
   (Caddy → 127.0.0.1:3002), otherwise `RemoteAddr`. Otherwise the header can be forged.
2. Read `X-HWID`, `X-Device-OS`, `X-Ver-OS`, `X-Device-Model`, `User-Agent`
   (truncated to 256 characters).
3. Write to `devices` for the subscription user — regardless of whether that user is
   online in xray or not.
4. Log write errors, but do not break the subscription.

### API

`GET /api/devices` already returns `Username, IP, FirstSeen, LastSeen, online` —
we add `hwid, platform, os_version, model, user_agent`.

### UI

In the “HWID / devices” panel the columns are filled from the API; an empty value
is shown as “—”. The “xray only returns the name and IP…” caption is removed.

## Security and privacy

- Headers are client data and cannot be trusted: they are used for display only,
  not for authorization and not for limits.
- `X-Forwarded-For` is accepted only from the loopback proxy.
- Lengths are bounded, and values reach HTML through Svelte's normal escaped output.

## Verification

- store test: an upsert with headers fills the fields; a repeated upsert without
  headers does not erase them; a new IP creates a row.
- sub test: `GET /sub/{token}` with a set of headers writes the device; without
  headers — only the IP.
- API test: `/api/devices` contains the new fields.
- Manual check: import the subscription in Happ/INCY on a tablet → the panel shows
  HWID, platform, model, UA.
- `go test ./...`, `npm run check`, `npx vitest run`.

## Out of scope

- Device limit by HWID instead of IP (`device_limit` stays by IP).
- Device change history, blocking by HWID, geolocation.
- Headers the client did not send stay empty.

## Open questions

1. Should an entry with the same IP but a different HWID count as a separate HWID
   device (the key is currently user+IP)? Option: key (user, HWID) with a fallback to IP.
2. Is a separate “all devices” screen with a per-user filter needed, or is the
   current view from the “Users ▾” menu enough.
