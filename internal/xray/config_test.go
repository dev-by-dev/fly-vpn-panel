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
	got, err := GenerateConfig(testInbounds(), testUsers(), testRoutingJSON, testEgressIP, testAPIPort)
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
	cfg, err := GenerateConfig(inbounds, nil, "", "", testAPIPort)
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
	cfg, err := GenerateConfig(testInbounds(), users, "", "", testAPIPort)
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

func TestGenerateNoSanitizer(t *testing.T) {
	routing := `{"domainStrategy":"IPIfNonMatch","rules":[
		{"type":"field","domain":["geosite:category-ads-all","domain:example.com"],"outboundTag":"block"},
		{"type":"field","ip":["203.0.113.0/24","geoip:private"],"outboundTag":"block"}]}`
	cfg, err := GenerateConfig(testInbounds(), nil, routing, "", testAPIPort)
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	got := unmarshalAny(t, cfg, "generated").(map[string]any)["routing"]
	if !reflect.DeepEqual(got, unmarshalAny(t, []byte(routing), "input routing")) {
		t.Errorf("routing was altered:\ngot  %v\nwant %v", got, routing)
	}
}

func TestGenerateEmptyRouting(t *testing.T) {
	cfg, err := GenerateConfig(testInbounds(), nil, "", "", testAPIPort)
	if err != nil {
		t.Fatalf("GenerateConfig(): %v", err)
	}
	got := unmarshalAny(t, cfg, "generated").(map[string]any)["routing"]
	want := unmarshalAny(t, []byte(`{"domainStrategy":"AsIs","rules":[]}`), "default routing")
	if !reflect.DeepEqual(got, want) {
		t.Errorf("empty routingJSON: got %v, want default %v", got, want)
	}
}

func TestGenerateBadRouting(t *testing.T) {
	if _, err := GenerateConfig(testInbounds(), nil, "{not json", "", testAPIPort); err == nil {
		t.Error("GenerateConfig(bad routing) = nil error, want error")
	}
}

func TestGenerateNoEgress(t *testing.T) {
	cfg, err := GenerateConfig(testInbounds(), nil, "", "", testAPIPort)
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
	cfg, err := GenerateConfig(testInbounds(), testUsers(), testRoutingJSON, testEgressIP, testAPIPort)
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
