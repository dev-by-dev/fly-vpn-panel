package sub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"flypanel/internal/store"
	"flypanel/internal/users"
)

// All fixtures are TEST-NET (203.0.113.x), example.com domains and
// fictional usernames. www.microsoft.com:443 is the spec's Reality dest
// constant. No real infrastructure appears anywhere.

const (
	testEntryIP = "203.0.113.10" // TEST-NET-3
	testUUID    = "123e4567-e89b-42d3-a456-426614174000"
)

func testInbound(tag string, port int, pbk, sid, sni, path, mode, fp string) store.Inbound {
	cfg := map[string]any{
		"port": port,
		"tag":  tag,
		"streamSettings": map[string]any{
			"network":  "xhttp",
			"security": "reality",
			"realitySettings": map[string]any{
				"dest":        "www.microsoft.com:443",
				"serverNames": []any{sni, "alt." + sni},
				"privateKey":  "priv_" + tag,
				"publicKey":   pbk,
				"shortIds":    []any{sid, sid + "2"},
			},
			"xhttpSettings": map[string]any{"path": path, "mode": mode},
		},
	}
	if fp != "" {
		rs := cfg["streamSettings"].(map[string]any)["realitySettings"].(map[string]any)
		rs["fingerprint"] = fp
	}
	raw, _ := json.Marshal(cfg)
	return store.Inbound{Tag: tag, Enabled: true, ConfigJSON: string(raw)}
}

func inboundA() store.Inbound {
	return testInbound("in-a", 8443, "pbk_a", "sid_a", "a.example.com", "/xa", "auto", "")
}
func inboundB() store.Inbound {
	return testInbound("in-b", 9443, "pbk_b", "sid_b", "c.example.com", "/xb", "packet-up", "firefox")
}

func testUser() store.User {
	return store.User{
		Username: "testuser", UUID: testUUID, SubToken: "tok1234567890abc",
		Expiry: time.Now().Add(24 * time.Hour).UTC(), QuotaBytes: 1000,
	}
}

func newTestServer(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	h := &Handlers{Store: st, EntryIP: testEntryIP}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /sub/{token}", h.Sub)
	mux.HandleFunc("GET /sub/{token}/{format}", h.Sub)
	mux.HandleFunc("GET /api/templates/{format}", h.TemplateGet)
	mux.HandleFunc("PUT /api/templates/{format}", h.TemplatePut)
	return st, mux
}

func do(t *testing.T, mux http.Handler, method, path, ua, body string) *http.Response {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rdr)
	if ua != "" {
		req.Header.Set("User-Agent", ua)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Result()
}

func TestRenderBase64(t *testing.T) {
	inC := inboundA()
	inC.Tag, inC.Enabled = "in-c", false // disabled: skipped
	out := RenderBase64([]store.Inbound{inboundA(), inboundB(), inC}, testUser(), testEntryIP)

	raw, err := base64.StdEncoding.DecodeString(out)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines (disabled inbound skipped), got %d: %q", len(lines), raw)
	}
	for i, wantTag := range []string{"in-a", "in-b"} {
		u, err := url.Parse(lines[i])
		if err != nil {
			t.Fatalf("parse link %d: %v", i, err)
		}
		if u.Scheme != "vless" {
			t.Errorf("line %d: scheme = %q, want vless", i, u.Scheme)
		}
		wantPort := map[string]int{"in-a": 8443, "in-b": 9443}[wantTag]
		if u.Host != fmt.Sprintf("%s:%d", testEntryIP, wantPort) {
			t.Errorf("line %d: host = %q, want %s:%d", i, u.Host, testEntryIP, wantPort)
		}
		q := u.Query()
		// every query parameter pinned per inbound:
		exp := map[string]string{"type": "xhttp", "security": "reality", "encryption": "none"}
		switch wantTag {
		case "in-a":
			exp["path"], exp["mode"] = "/xa", "auto"
			exp["pbk"], exp["sid"], exp["sni"] = "pbk_a", "sid_a", "a.example.com"
			exp["fp"] = "chrome" // absent in config → default
		case "in-b":
			exp["path"], exp["mode"] = "/xb", "packet-up"
			exp["pbk"], exp["sid"], exp["sni"] = "pbk_b", "sid_b", "c.example.com"
			exp["fp"] = "firefox"
		}
		for k, v := range exp {
			if got := q.Get(k); got != v {
				t.Errorf("line %d: query %s = %q, want %q", i, k, got, v)
			}
		}
		if u.User.Username() != testUUID {
			t.Errorf("line %d: uuid = %q, want %q", i, u.User.Username(), testUUID)
		}
		if u.Fragment != wantTag {
			t.Errorf("line %d: fragment = %q, want %q", i, u.Fragment, wantTag)
		}
	}
}

func TestRenderJSONArray(t *testing.T) {
	tpl := DefaultTemplate("json")
	if tpl == "" {
		t.Fatal("no embedded json default template")
	}
	out, err := RenderJSON([]store.Inbound{inboundA(), inboundB()}, testUser(), testEntryIP, tpl)
	if err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	if !json.Valid(out) {
		t.Fatalf("rendered output is not valid JSON: %.200s", out)
	}
	var arr []map[string]any
	if err := json.Unmarshal(out, &arr); err != nil {
		t.Fatalf("unmarshal array: %v", err)
	}
	if len(arr) != 2 {
		t.Fatalf("want array len 2, got %d", len(arr))
	}
	// decode a reference of the template by substituting dummy values
	// (string placeholders sit inside JSON quotes, {{PORT}} is bare)
	ref := tpl
	for _, k := range []string{"UUID", "ADDRESS", "PBK", "SID", "SNI", "FP", "PATH", "MODE", "TAG"} {
		ref = strings.ReplaceAll(ref, "{{"+k+"}}", "x")
	}
	ref = strings.ReplaceAll(ref, "{{PORT}}", "443")
	var tplObj map[string]any
	if err := json.Unmarshal([]byte(ref), &tplObj); err != nil {
		t.Fatalf("unmarshal template reference: %v", err)
	}
	for i, tag := range []string{"in-a", "in-b"} {
		// routing/dns/inbounds preserved verbatim from the template (anti-sanitizer)
		for _, key := range []string{"routing", "dns", "inbounds"} {
			if !reflect.DeepEqual(arr[i][key], tplObj[key]) {
				t.Errorf("config %d: %s differs from template", i, key)
			}
		}
		obs, _ := arr[i]["outbounds"].([]any)
		first, _ := obs[0].(map[string]any)
		if first["tag"] != tag {
			t.Errorf("config %d: outbound tag = %v, want %q", i, first["tag"], tag)
		}
		st, _ := first["streamSettings"].(map[string]any)
		if st["network"] != "xhttp" || st["security"] != "reality" {
			t.Errorf("config %d: streamSettings = %v", i, st)
		}
		rs, _ := st["realitySettings"].(map[string]any)
		if rs["publicKey"] != "pbk_"+tag[len(tag)-1:] || rs["shortId"] != "sid_"+tag[len(tag)-1:] {
			t.Errorf("config %d: realitySettings = %v", i, rs)
		}
	}
	vnext, _, _ := pickVnext(arr[0])
	if vnext["address"] != testEntryIP || vnext["port"] != float64(8443) {
		t.Errorf("config 0: vnext = %v", vnext)
	}

	s := string(out)
	for _, verbatim := range []string{
		`"ext:geosite.dat:category-ads-all"`, `"full:app-measurement.com"`,
		`"full:ads.youtube.com"`, `"domain:vk.com"`, `"domain:fastpic.org"`,
		`"regexp:\\.ru$"`, `"bittorrent"`, `"geoip:ru"`,
	} {
		if !strings.Contains(s, verbatim) {
			t.Errorf("rendered config lost verbatim rule %s", verbatim)
		}
	}

	// leftover placeholder → error
	if _, err := RenderJSON([]store.Inbound{inboundA()}, testUser(), testEntryIP, `{"x":"{{UUID}}","y":{{TYPO}}}`); err == nil {
		t.Error("leftover placeholder must error")
	}
}

func pickVnext(cfg map[string]any) (vnext map[string]any, user map[string]any, ok bool) {
	obs, _ := cfg["outbounds"].([]any)
	first, _ := obs[0].(map[string]any)
	settings, _ := first["settings"].(map[string]any)
	vnexts, _ := settings["vnext"].([]any)
	if len(vnexts) == 0 {
		return nil, nil, false
	}
	vnext, _ = vnexts[0].(map[string]any)
	users, _ := vnext["users"].([]any)
	user, _ = users[0].(map[string]any)
	return vnext, user, true
}

func TestRenderYAML(t *testing.T) {
	tpl := DefaultTemplate("mihomo")
	if tpl == "" {
		t.Fatal("no embedded mihomo default template")
	}
	params, err := proxyParams([]store.Inbound{inboundA(), inboundB()}, testUser(), testEntryIP)
	if err != nil {
		t.Fatalf("proxyParams: %v", err)
	}
	out, err := RenderYAML(tpl, params)
	if err != nil {
		t.Fatalf("RenderYAML: %v", err)
	}
	if strings.Contains(out, "{{") {
		t.Error("unresolved placeholder left in output")
	}
	for _, verbatim := range []string{
		// prod-mirrored rules must survive rendering verbatim
		"ipv6: false", "sniffer:", "override-destination: true",
		"DOMAIN-SUFFIX,ru,DIRECT", "DOMAIN-SUFFIX,su,DIRECT", "DOMAIN-SUFFIX,xn--p1ai,DIRECT",
		"DOMAIN-SUFFIX,vk.com,DIRECT", "DOMAIN-SUFFIX,userapi.com,DIRECT",
		"DOMAIN-SUFFIX,yandex.net,DIRECT", "DOMAIN-SUFFIX,mycdn.me,DIRECT",
		"DOMAIN-SUFFIX,2gis.com,DIRECT", "DOMAIN-SUFFIX,kudago.com,DIRECT",
		"DOMAIN-SUFFIX,fastpic.org,DIRECT",
		"IP-CIDR,91.108.4.0/22,proxy,no-resolve", "IP-CIDR,91.108.8.0/21,proxy,no-resolve",
		"IP-CIDR,91.108.16.0/20,proxy,no-resolve", "IP-CIDR,91.108.56.0/22,proxy,no-resolve",
		"IP-CIDR,149.154.160.0/20,proxy,no-resolve", "IP-CIDR,95.161.64.0/20,proxy,no-resolve",
		"GEOIP,RU,DIRECT", "MATCH,proxy",
		"anti-ad.net/clash.yaml", "RULE-SET,reject-ads,REJECT",
		"DOMAIN-SUFFIX,ads.youtube.com,REJECT", "DOMAIN-SUFFIX,app-measurement.com,REJECT",
		"DOMAIN-SUFFIX,ads.ytimg.com,REJECT", "DOMAIN-SUFFIX,googleadservices.com,REJECT",
		// rendered proxy entries + group membership
		`name: "in-a"`, `name: "in-b"`, "type: vless", "network: xhttp", "tls: true", "udp: true",
		"reality-opts: {public-key: \"pbk_a\", short-id: \"sid_a\"}",
		`xhttp-opts: {path: "/xa", mode: "auto"}`, "client-fingerprint: \"chrome\"",
		`proxies: ["in-a", "in-b"]`, `server: "203.0.113.10"`, "uuid: \"" + testUUID + "\"",
		`servername: "a.example.com"`,
	} {
		if !strings.Contains(out, verbatim) {
			t.Errorf("rendered yaml lost %q", verbatim)
		}
	}
	if strings.Count(out, "ipv6: false") < 2 {
		t.Error("ipv6: false must appear both top-level and in dns")
	}
	// leftover placeholder → error
	if _, err := RenderYAML("a: {{PROXIES}}\nb: {{OOPS}}", params); err == nil {
		t.Error("leftover placeholder must error")
	}
}

func TestFormatForUA(t *testing.T) {
	cases := []struct{ ua, want string }{
		{"v2rayNG/1.9.30", "base64"},
		{"FlClash/0.9.3 clash-verge", "mihomo"},
		{"clash-verge/2.5.10", "mihomo"},
		{"ClashVerge/2.2.3", "base64"},
		{"clash.meta", "base64"},
		{"Mozilla/5.0", "base64"},
		{"", "base64"},
	}
	for _, c := range cases {
		if got := FormatForUA(c.ua); got != c.want {
			t.Errorf("FormatForUA(%q) = %q, want %q", c.ua, got, c.want)
		}
	}
}

func TestSubAccess(t *testing.T) {
	st, mux := newTestServer(t)

	// unknown token vs deleted token: identical 404 body (no oracle)
	res := do(t, mux, "GET", "/sub/unknowntoken1", "", "")
	if res.StatusCode != 404 {
		t.Fatalf("unknown token: status = %d, want 404", res.StatusCode)
	}
	unknownBody, _ := io.ReadAll(res.Body)
	if err := st.UserCreate(&store.User{Username: "gone", UUID: testUUID, SubToken: "deletedtoken1", Expiry: time.Now().Add(time.Hour).UTC(), CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	gone, _ := st.UserGetByUsername("gone")
	if err := st.UserDelete(gone.ID); err != nil {
		t.Fatal(err)
	}
	res = do(t, mux, "GET", "/sub/deletedtoken1", "", "")
	if res.StatusCode != 404 {
		t.Fatalf("deleted token: status = %d, want 404", res.StatusCode)
	}
	deletedBody, _ := io.ReadAll(res.Body)
	if string(unknownBody) != string(deletedBody) {
		t.Errorf("404 bodies differ: unknown=%q deleted=%q", unknownBody, deletedBody)
	}
	if !strings.Contains(string(unknownBody), `"error":"not found"`) {
		t.Errorf("404 body = %q", unknownBody)
	}

	// expired → disabled in DB + 403 expired
	exp := &store.User{Username: "expireduser", UUID: testUUID, SubToken: "expiredtoken1", Expiry: time.Now().Add(-time.Minute).UTC(), CreatedAt: time.Now().UTC()}
	if err := st.UserCreate(exp); err != nil {
		t.Fatal(err)
	}
	res = do(t, mux, "GET", "/sub/expiredtoken1", "", "")
	if res.StatusCode != 403 {
		t.Fatalf("expired: status = %d, want 403", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), `"error":"expired"`) {
		t.Errorf("expired body = %q", body)
	}
	after, err := st.UserGet(exp.ID)
	if err != nil || !after.Disabled || after.DisabledReason != "expired" {
		t.Errorf("expired user not disabled in DB: %+v err=%v", after, err)
	}

	// disabled → 403 disabled
	dis := &store.User{Username: "disableduser", UUID: testUUID, SubToken: "disabledtok1", Expiry: time.Now().Add(time.Hour).UTC(), Disabled: true, DisabledReason: "manual", CreatedAt: time.Now().UTC()}
	if err := st.UserCreate(dis); err != nil {
		t.Fatal(err)
	}
	res = do(t, mux, "GET", "/sub/disabledtok1", "", "")
	if res.StatusCode != 403 {
		t.Fatalf("disabled: status = %d, want 403", res.StatusCode)
	}
	body, _ = io.ReadAll(res.Body)
	if !strings.Contains(string(body), `"error":"disabled"`) {
		t.Errorf("disabled body = %q", body)
	}

	// active user: json suffix, samples header, valid array
	u := &store.User{Username: "activeuser", UUID: testUUID, SubToken: "activetoken1", Expiry: time.Now().Add(24 * time.Hour).UTC(), QuotaBytes: 1000, CreatedAt: time.Now().UTC()}
	if err := st.UserCreate(u); err != nil {
		t.Fatal(err)
	}
	if err := st.SampleInsert("activeuser", time.Now().UTC(), 100, 200); err != nil {
		t.Fatal(err)
	}
	in := inboundA()
	if err := st.InboundCreate(&in); err != nil {
		t.Fatal(err)
	}
	res = do(t, mux, "GET", "/sub/activetoken1/json", "", "")
	if res.StatusCode != 200 {
		t.Fatalf("active json: status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("json content-type = %q", ct)
	}
	wantHdr := fmt.Sprintf("upload=100; download=200; total=1000; expire=%d", u.Expiry.Unix())
	if got := res.Header.Get("Subscription-Userinfo"); got != wantHdr {
		t.Errorf("Subscription-Userinfo = %q, want %q", got, wantHdr)
	}
	body, _ = io.ReadAll(res.Body)
	var arr []any
	if err := json.Unmarshal(body, &arr); err != nil || len(arr) != 1 {
		t.Errorf("json body not a 1-element array: %v %.200s", err, body)
	}

	// UA fallback: FlClash on the bare URL → mihomo yaml
	res = do(t, mux, "GET", "/sub/activetoken1", "FlClash/0.9.3 clash-verge", "")
	if res.StatusCode != 200 {
		t.Fatalf("active mihomo: status = %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "text/yaml" {
		t.Errorf("mihomo content-type = %q", ct)
	}
	body, _ = io.ReadAll(res.Body)
	if !strings.Contains(string(body), "proxies:") || !strings.Contains(string(body), `name: "in-a"`) {
		t.Errorf("mihomo body missing rendered proxies: %.200s", body)
	}

	// unknown format suffix → 400
	res = do(t, mux, "GET", "/sub/activetoken1/xml", "", "")
	if res.StatusCode != 400 {
		t.Errorf("bad format suffix: status = %d, want 400", res.StatusCode)
	}
}

// TestSubManualDisabledPastExpiry: a manually disabled user whose expiry
// has passed must keep disabled_reason "manual" — re-tagging to
// "expired" would let the poller auto-enable them once the expiry is
// extended (Decide enables only expired-tagged users).
func TestSubManualDisabledPastExpiry(t *testing.T) {
	st, mux := newTestServer(t)

	u := &store.User{
		Username: "manualuser", UUID: testUUID, SubToken: "manualtoken1",
		Expiry: time.Now().Add(-time.Hour).UTC(), Disabled: true, DisabledReason: "manual",
		CreatedAt: time.Now().UTC(),
	}
	if err := st.UserCreate(u); err != nil {
		t.Fatal(err)
	}

	res := do(t, mux, "GET", "/sub/manualtoken1", "", "")
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("manual-disabled past-expiry: status = %d, want 403", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), `"error":"disabled"`) {
		t.Errorf("body = %q, want disabled", body)
	}
	after, err := st.UserGet(u.ID)
	if err != nil || after.DisabledReason != "manual" {
		t.Fatalf("reason after fetch = %q (err %v), want manual untouched", after.DisabledReason, err)
	}

	// Extending the expiry must not resurrect the user: the row stays
	// disabled and Decide never emits enable for a manual disable.
	after.Expiry = time.Now().Add(24 * time.Hour).UTC()
	if err := st.UserUpdate(after); err != nil {
		t.Fatal(err)
	}
	fresh, err := st.UserGet(u.ID)
	if err != nil || !fresh.Disabled {
		t.Fatalf("user after expiry extension: disabled=%v (err %v), want still disabled", fresh.Disabled, err)
	}
	for _, a := range users.Decide(*fresh, 0, nil, time.Now().UTC()) {
		if a.Type == "enable" {
			t.Fatalf("Decide emits enable for a manual disable: %+v", a)
		}
	}
}

// TestRenderJSONSinglePass: substitution must be a single pass. A value
// containing another placeholder ({{TAG}} inside the path) must never be
// re-substituted with that key's value — it surfaces as an unresolved
// placeholder instead. Map iteration made this order-dependent, so every
// attempt must behave identically.
func TestRenderJSONSinglePass(t *testing.T) {
	in := testInbound("in-a", 8443, "pbk_a", "sid_a", "a.example.com", "/x-{{TAG}}", "auto", "")
	for i := 0; i < 20; i++ {
		_, err := RenderJSON([]store.Inbound{in}, testUser(), testEntryIP, DefaultTemplate("json"))
		if err == nil {
			t.Fatalf("attempt %d: placeholder inside a value was re-substituted (multi-pass)", i)
		}
	}
}

func TestTemplatePutNoSanitizer(t *testing.T) {
	_, mux := newTestServer(t)

	// default present before any PUT
	res := do(t, mux, "GET", "/api/templates/mihomo", "", "")
	if res.StatusCode != 200 {
		t.Fatalf("template get: status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "GEOIP,RU,DIRECT") {
		t.Errorf("default mihomo template missing RU rules: %.200s", body)
	}

	custom := "mixed-port: 7890\nmode: rule\nipv6: false\nproxies:\n{{PROXIES}}\nproxy-groups:\n  - name: 'proxy'\n    type: 'select'\n    proxies: {{PROXY_NAMES}}\nrules:\n  - DOMAIN-SUFFIX,customrule.example,DIRECT\n  - IP-CIDR,198.51.100.0/24,proxy\n  - MATCH,proxy\n"
	res = do(t, mux, "PUT", "/api/templates/mihomo", "", custom)
	if res.StatusCode != 200 {
		t.Fatalf("template put: status = %d", res.StatusCode)
	}
	res = do(t, mux, "GET", "/api/templates/mihomo", "", "")
	if res.StatusCode != 200 {
		t.Fatalf("template re-get: status = %d", res.StatusCode)
	}
	body, _ = io.ReadAll(res.Body)
	if string(body) != custom {
		t.Errorf("template not stored verbatim:\n got=%q\nwant=%q", body, custom)
	}

	// broken placeholder → 400, stored template untouched
	broken := strings.Replace(custom, "{{PROXIES}}", "{{TYPO}}", 1)
	res = do(t, mux, "PUT", "/api/templates/mihomo", "", broken)
	if res.StatusCode != 400 {
		t.Errorf("broken template: status = %d, want 400", res.StatusCode)
	}
	res = do(t, mux, "GET", "/api/templates/mihomo", "", "")
	body, _ = io.ReadAll(res.Body)
	if string(body) != custom {
		t.Error("broken PUT must not overwrite the stored template")
	}

	// formats without templates rejected
	for _, f := range []string{"base64", "bogus"} {
		if res := do(t, mux, "PUT", "/api/templates/"+f, "", "x"); res.StatusCode != 400 {
			t.Errorf("PUT %s: status = %d, want 400", f, res.StatusCode)
		}
	}
}
