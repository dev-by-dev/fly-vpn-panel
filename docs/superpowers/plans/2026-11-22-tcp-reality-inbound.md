# VLESS+TCP+Reality inbound and subscriptions for any client

## Task

The panel profile is VLESS+XHTTP+Reality. On the tablet (Huawei, Android 13) no
client accepts it:

- Incy 3.9.9 discards `type=xhttp` on import and saves VLESS·TCP without TLS;
- Hiddify 5.5.5-dev sends not a single packet when connecting;
- v2rayNG 2.3.15 parses the profile correctly, but its core pushes outbound
  traffic into its own tun0 (not a byte reaches the server);
- Happ crashes while assembling REALITY: `empty "serverNames"`.

The server and the link are fine, though: real Xray 26.11.9 with the same config
goes through the tunnel successfully (api.ipify.org → 203.0.113.9).

## Solution

1. Add a second inbound, VLESS+TCP+Reality (port 8444) — a transport all the
   listed clients understand.
2. Teach the subscription renderers networks other than XHTTP:
   - `links.go`: `linkParams.Net` from `streamSettings.network` (default
     `xhttp` — backwards compatibility); `xhttpSettings` is mandatory only for
     xhttp; the link gets `type=<net>`, while `path`/`mode` only for xhttp.
   - `yaml.go`: `network: <net>`, `xhttp-opts` only for xhttp.
   - `json.go` + `default_json.json`: the `{{NETWORK}}` placeholder.

## Verification

- `go test ./internal/sub/` — a new `TestRenderBase64TCP`: two base64 lines,
  xhttp keeps `path`/`mode`, tcp has `type=tcp`, `security=reality`,
  `encryption=none`, `pbk/sid/sni`, `fp=chrome`, without `path`/`mode`.
- `go test ./...` — green.
- After deploy: the subscription contains two links, the TCP inbound listens on
  8444, and connecting from the tablet works.

## Out of scope

- A JSON template for the TCP inbound: the xhttp template stays (clients for TCP
  use base64/mihomo).