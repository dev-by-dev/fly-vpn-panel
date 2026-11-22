package xray

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"flypanel/internal/store"
)

const (
	testAPIPort  = 15490
	testEgressIP = "198.51.100.7"

	// Throwaway X25519 pair generated for this fixture; the public key is
	// what a client subscription would carry as pbk.
	testRealityPriv = "X7vxaP7lfg4MHIF3sVKU--A3A8bqs6o-Ja_IN-A69WU"
	testRealityPub  = "iwqFCsFx8i7Od-iStEG9TuxQGM6O_rR1Vo3NPrcZaW0"
)

// VLESS + XHTTP + Reality inbound fixture. TEST-NET/example values only.
const testInboundJSON = `{
  "tag": "vless-in",
  "listen": "0.0.0.0",
  "port": 8443,
  "protocol": "vless",
  "settings": {"clients": [], "decryption": "none"},
  "streamSettings": {
    "network": "xhttp",
    "security": "reality",
    "xhttpSettings": {"path": "/xhttp", "mode": "stream-up"},
    "realitySettings": {
      "show": false,
      "dest": "www.microsoft.com:443",
      "xver": 0,
      "serverNames": ["www.microsoft.com"],
      "privateKey": "` + testRealityPriv + `",
      "shortIds": ["0123456789abcdef"]
    }
  }
}`

const testRoutingJSON = `{"domainStrategy":"AsIs","rules":[{"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"block"}]}`

func testInbounds() []store.Inbound {
	return []store.Inbound{
		{ID: 1, Tag: "vless-in", Enabled: true, ConfigJSON: testInboundJSON, CreatedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
	}
}

func testUsers() []store.User {
	return []store.User{
		{ID: 1, Username: "alice", UUID: "11111111-1111-4111-8111-111111111111", CreatedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Username: "bob", UUID: "22222222-2222-4222-8222-222222222222", CreatedAt: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
	}
}

// unmarshalAny parses JSON into any, failing the test on error.
func unmarshalAny(t *testing.T, data []byte, what string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal %s: %v\n%s", what, err, data)
	}
	return v
}

// vlessClients extracts the clients array of the "vless-in" inbound.
func vlessClients(t *testing.T, doc any) []any {
	t.Helper()
	inbounds, ok := doc.(map[string]any)["inbounds"].([]any)
	if !ok {
		t.Fatalf("inbounds missing in generated config: %v", doc)
	}
	for _, ib := range inbounds {
		m, ok := ib.(map[string]any)
		if !ok || m["tag"] != "vless-in" {
			continue
		}
		settings, ok := m["settings"].(map[string]any)
		if !ok {
			t.Fatalf("vless-in settings missing: %v", m)
		}
		clients, ok := settings["clients"].([]any)
		if !ok {
			t.Fatalf("vless-in clients missing: %v", settings)
		}
		return clients
	}
	t.Fatal("vless-in inbound not found")
	return nil
}

func TestGenerateGolden(t *testing.T) {
	got, err := GenerateConfig(testInbounds(), testUsers(), testRoutingJSON, testEgressIP, testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "golden.json"))
	if err != nil {
		t.Fatalf("read golden.json: %v", err)
	}
	if !reflect.DeepEqual(unmarshalAny(t, got, "generated"), unmarshalAny(t, want, "golden")) {
		t.Errorf("generated config != golden.json (semantic comparison)")
	}
}

// TestGenerateVlessDecryption: xray 26.x rejects vless inbounds whose
// settings lack decryption:"none". GenerateConfig must inject it for
// vless inbounds only — never for other protocols (e.g. dokodemo-door).
// TestGenerateStatsObject: xray-core only loads the app/stats manager
// when the JSON config carries a top-level "stats" object; policy stats
// flags alone create no counters, so the poller would record nothing.
func TestGenerateStatsObject(t *testing.T) {
	cfg, err := GenerateConfig(testInbounds(), nil, "", "", testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	stats, ok := unmarshalAny(t, cfg, "generated").(map[string]any)["stats"].(map[string]any)
	if !ok || stats == nil {
		t.Errorf("top-level stats = %v, want non-null object so the stats manager loads", stats)
	}
}

func TestGenerateVlessDecryption(t *testing.T) {
	inbounds := []store.Inbound{
		{ID: 1, Tag: "vless-nodec", Enabled: true, ConfigJSON: `{
			"tag": "vless-nodec", "listen": "0.0.0.0", "port": 8444,
			"protocol": "vless", "settings": {"clients": []},
			"streamSettings": {"network": "tcp"}
		}`, CreatedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
		{ID: 2, Tag: "dokodemo-in", Enabled: true, ConfigJSON: `{
			"tag": "dokodemo-in", "listen": "127.0.0.1", "port": 8445,
			"protocol": "dokodemo-door", "settings": {"address": "127.0.0.1"}
		}`, CreatedAt: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)},
	}
	cfg, err := GenerateConfig(inbounds, nil, "", "", testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	got := map[string]map[string]any{}
	for _, ib := range unmarshalAny(t, cfg, "generated").(map[string]any)["inbounds"].([]any) {
		m := ib.(map[string]any)
		got[m["tag"].(string)] = m["settings"].(map[string]any)
	}
	if got["vless-nodec"]["decryption"] != "none" {
		t.Errorf("vless settings.decryption = %v, want \"none\"", got["vless-nodec"]["decryption"])
	}
	if _, ok := got["dokodemo-in"]["decryption"]; ok {
		t.Errorf("dokodemo-door settings has decryption %v, want none injected", got["dokodemo-in"]["decryption"])
	}
}

func TestGenerateInjectsOnlyActive(t *testing.T) {
	users := append(testUsers(), store.User{
		ID: 3, Username: "mallory", UUID: "33333333-3333-4333-8333-333333333333",
		Disabled: true, DisabledReason: "banned", CreatedAt: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC),
	})
	cfg, err := GenerateConfig(testInbounds(), users, "", "", testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	clients := vlessClients(t, unmarshalAny(t, cfg, "generated"))
	if len(clients) != 2 {
		t.Fatalf("len(clients) = %d, want 2 (disabled user must be excluded): %v", len(clients), clients)
	}
	for _, c := range clients {
		if c.(map[string]any)["email"] == "mallory" {
			t.Errorf("disabled user mallory present in clients: %v", c)
		}
	}
}

// apiRoutingRules extracts routing.rules as maps, failing the test when
// the array is missing.
func apiRoutingRules(t *testing.T, cfg []byte) []map[string]any {
	t.Helper()
	routing, ok := unmarshalAny(t, cfg, "generated").(map[string]any)["routing"].(map[string]any)
	if !ok {
		t.Fatalf("routing missing in generated config")
	}
	raw, ok := routing["rules"].([]any)
	if !ok {
		t.Fatalf("routing.rules missing: %v", routing)
	}
	rules := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		rules = append(rules, r.(map[string]any))
	}
	return rules
}

// isAPIRule reports whether a rule routes the "api" inbound to the "api"
// outbound.
func isAPIRule(r map[string]any) bool {
	if r["outboundTag"] != "api" {
		return false
	}
	tags, _ := r["inboundTag"].([]any)
	for _, tag := range tags {
		if tag == "api" {
			return true
		}
	}
	return false
}

// TestGenerateAPIRoutingRule: gRPC traffic on the dokodemo-door "api"
// inbound must be routed to the api service. Without the api->api rule it
// falls through to the freedom outbound, loops back to 127.0.0.1 and the
// connection closes ("error reading server preface"), so the stats poller
// cannot reach the API.
func TestGenerateAPIRoutingRule(t *testing.T) {
	wantAPIRule := map[string]any{"type": "field", "inboundTag": []any{"api"}, "outboundTag": "api"}
	userDomainRule := map[string]any{
		"type": "field", "domain": []any{"geosite:category-ads-all"}, "outboundTag": "block",
	}
	apiInUserRouting := `{"domainStrategy":"AsIs","rules":[
		{"type":"field","inboundTag":["api"],"outboundTag":"api"},
		{"type":"field","domain":["geosite:category-ads-all"],"outboundTag":"block"}]}`

	tests := []struct {
		name        string
		routingJSON string
		check       func(t *testing.T, rules []map[string]any)
	}{
		{
			name:        "empty routing gets exactly the api rule",
			routingJSON: "",
			check: func(t *testing.T, rules []map[string]any) {
				if len(rules) != 1 || !reflect.DeepEqual(rules[0], wantAPIRule) {
					t.Errorf("rules = %v, want exactly [%v]", rules, wantAPIRule)
				}
			},
		},
		{
			name:        "user rules preserved, api rule prepended",
			routingJSON: testRoutingJSON,
			check: func(t *testing.T, rules []map[string]any) {
				if len(rules) != 2 {
					t.Fatalf("len(rules) = %d, want 2: %v", len(rules), rules)
				}
				if !reflect.DeepEqual(rules[0], wantAPIRule) {
					t.Errorf("rules[0] = %v, want api rule %v (must be prepended)", rules[0], wantAPIRule)
				}
				if !reflect.DeepEqual(rules[1], userDomainRule) {
					t.Errorf("rules[1] = %v, want user rule %v preserved", rules[1], userDomainRule)
				}
			},
		},
		{
			name:        "existing api rule not duplicated",
			routingJSON: apiInUserRouting,
			check: func(t *testing.T, rules []map[string]any) {
				n := 0
				for _, r := range rules {
					if isAPIRule(r) {
						n++
					}
				}
				if n != 1 {
					t.Errorf("api->api rule count = %d, want 1: %v", n, rules)
				}
				if len(rules) != 2 {
					t.Errorf("len(rules) = %d, want 2 (no duplicate added): %v", len(rules), rules)
				}
				if !reflect.DeepEqual(rules[1], userDomainRule) {
					t.Errorf("rules[1] = %v, want user rule %v preserved", rules[1], userDomainRule)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := GenerateConfig(testInbounds(), nil, tt.routingJSON, "", testAPIPort, "")
			if err != nil {
				t.Fatalf("GenerateConfig(): %v", err)
			}
			tt.check(t, apiRoutingRules(t, cfg))
		})
	}
}

func TestGenerateNoSanitizer(t *testing.T) {
	routing := `{"domainStrategy":"IPIfNonMatch","rules":[
		{"type":"field","domain":["geosite:category-ads-all","domain:example.com"],"outboundTag":"block"},
		{"type":"field","ip":["203.0.113.0/24","geoip:private"],"outboundTag":"block"}]}`
	cfg, err := GenerateConfig(testInbounds(), nil, routing, "", testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	got := unmarshalAny(t, cfg, "generated").(map[string]any)["routing"].(map[string]any)
	gotRules, _ := got["rules"].([]any)
	// User rules must survive verbatim after the injected api rule.
	wantUser := unmarshalAny(t, []byte(routing), "input routing").(map[string]any)["rules"].([]any)
	if !reflect.DeepEqual(gotRules[1:], wantUser) {
		t.Errorf("user rules altered:\ngot  %v\nwant %v", gotRules[1:], wantUser)
	}
	if !isAPIRule(gotRules[0].(map[string]any)) {
		t.Errorf("rules[0] = %v, want injected api rule", gotRules[0])
	}
	if got["domainStrategy"] != "IPIfNonMatch" {
		t.Errorf("domainStrategy = %v, want IPIfNonMatch", got["domainStrategy"])
	}
}

func TestGenerateEmptyRouting(t *testing.T) {
	cfg, err := GenerateConfig(testInbounds(), nil, "", "", testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	got := unmarshalAny(t, cfg, "generated").(map[string]any)["routing"]
	want := unmarshalAny(t, []byte(`{"domainStrategy":"AsIs","rules":[
		{"type":"field","inboundTag":["api"],"outboundTag":"api"}]}`), "default routing")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("empty routingJSON: got %v, want default + api rule %v", got, want)
	}
}

func TestGenerateBadRouting(t *testing.T) {
	if _, err := GenerateConfig(testInbounds(), nil, "{not json", "", testAPIPort, ""); err == nil {
		t.Error("GenerateConfig(bad routing) = nil error, want error")
	}
}

// TestGenerateNullRouting: JSON null unmarshals into a nil map with a
// nil error; parseRouting must reject it, otherwise ensureAPIRule
// assigns into the nil map and panics.
func TestGenerateNullRouting(t *testing.T) {
	if _, err := GenerateConfig(testInbounds(), nil, "null", "", testAPIPort, ""); err == nil {
		t.Fatal("GenerateConfig(routing null) = nil error, want error (must be a JSON object)")
	}
}

func TestGenerateNoEgress(t *testing.T) {
	cfg, err := GenerateConfig(testInbounds(), nil, "", "", testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	outbounds := unmarshalAny(t, cfg, "generated").(map[string]any)["outbounds"].([]any)
	for _, ob := range outbounds {
		m := ob.(map[string]any)
		if m["tag"] != "direct" {
			continue
		}
		if m["protocol"] != "freedom" {
			t.Errorf("direct outbound protocol = %v, want freedom", m["protocol"])
		}
		if _, ok := m["sendThrough"]; ok {
			t.Errorf("empty egressIP: freedom outbound has sendThrough %v, want it omitted", m["sendThrough"])
		}
		return
	}
	t.Fatal("direct outbound not found")
}

func TestValidateIntegration(t *testing.T) {
	bin := os.Getenv("XRAY_BIN")
	if bin == "" {
		t.Skip("XRAY_BIN not set; skipping integration test with a real xray binary")
	}
	cfg, err := GenerateConfig(testInbounds(), testUsers(), testRoutingJSON, testEgressIP, testAPIPort, "")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	if err := Validate(bin, cfg); err != nil {
		t.Errorf("Validate(golden config) = %v, want nil", err)
	}
	if err := Validate(bin, []byte("{not json")); err == nil {
		t.Error("Validate(broken JSON) = nil, want error")
	}
}

func TestConfigXrayErrorLogPath(t *testing.T) {
	cfg, err := GenerateConfig(testInbounds(), nil, "", "", testAPIPort, "/opt/flypanel/data/xray-error.log")
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	var doc struct {
		Log struct {
			Loglevel string `json:"loglevel"`
			Error    string `json:"error"`
		} `json:"log"`
	}
	if err := json.Unmarshal(cfg, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Log.Error != "/opt/flypanel/data/xray-error.log" {
		t.Errorf("log.error = %q, want the given path", doc.Log.Error)
	}
	if doc.Log.Loglevel != "warning" {
		t.Errorf("log.loglevel = %q, want warning", doc.Log.Loglevel)
	}
}
