package api

import (
	"encoding/json"
	"testing"

	"flypanel/internal/store"
)

// TestSeedDefaultInboundVlessDecryption: the seeded vless inbound must
// carry decryption:"none" so `xray run -test` accepts the config as-is.
func TestSeedDefaultInboundVlessDecryption(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	defer st.Close()

	in, err := defaultInbound()
	if err != nil {
		t.Fatalf("defaultInbound(): %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(in.ConfigJSON), &cfg); err != nil {
		t.Fatalf("seeded config not JSON: %v", err)
	}
	if cfg["protocol"] != "vless" {
		t.Fatalf("seeded protocol = %v, want vless", cfg["protocol"])
	}
	settings, ok := cfg["settings"].(map[string]any)
	if !ok {
		t.Fatalf("seeded settings missing or not a map: %v", cfg["settings"])
	}
	if settings["decryption"] != "none" {
		t.Errorf("seeded vless settings.decryption = %v, want \"none\"", settings["decryption"])
	}
	if _, ok := settings["clients"].([]any); !ok {
		t.Errorf("seeded vless settings.clients = %T, want a JSON array", settings["clients"])
	}
}
