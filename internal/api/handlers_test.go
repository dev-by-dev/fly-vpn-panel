package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"flypanel/internal/auth"
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

// TestSettingsGetTotpEnabled: GET /api/settings must expose
// totp_enabled so the SPA renders the real 2FA state after a reload
// (instead of session-local state that starts disabled).
func TestSettingsGetTotpEnabled(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	defer st.Close()
	h := &Handlers{Store: st}

	get := func() map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
		rr := httptest.NewRecorder()
		h.SettingsGet(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("settings get: status %d, body %s", rr.Code, rr.Body.String())
		}
		var m map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
			t.Fatalf("decode settings: %v", err)
		}
		return m
	}

	if v, ok := get()["totp_enabled"].(bool); !ok || v {
		t.Fatalf("totp_enabled = %v, want false (2FA not enrolled)", get()["totp_enabled"])
	}
	if _, ok := get()["poll_interval_sec"]; !ok {
		t.Fatal("poll_interval_sec missing from settings GET")
	}

	if err := auth.AdminInit(st, "admin", "long-enough-password-1"); err != nil {
		t.Fatalf("AdminInit(): %v", err)
	}
	if err := st.AdminSetTOTP("JBSWY3DPEHPK3PXP", true); err != nil {
		t.Fatalf("AdminSetTOTP(): %v", err)
	}
	if v, ok := get()["totp_enabled"].(bool); !ok || !v {
		t.Fatalf("totp_enabled = %v, want true after enroll+verify", get()["totp_enabled"])
	}
}
