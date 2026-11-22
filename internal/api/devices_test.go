package api

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"flypanel/internal/store"
)

// Устройства за одним NAT делят IP: xray знает только пару (юзер, IP),
// поэтому зелёной должна быть ровно одна строка на пару.
func TestDevicesOneOnlinePerIP(t *testing.T) {
	h, st, j := newTestRouter(t)
	createAdmin(t, st, "correct horse battery")
	c := accessCookie(t, j)

	old := time.Now().Add(-2 * time.Hour)
	meta := func(hwid string) store.DeviceMeta {
		return store.DeviceMeta{HWID: hwid, UserAgent: "Happ/4.11.2"}
	}
	for _, hwid := range []string{"mac", "android"} {
		if err := st.DeviceUpsert("alice@example.com", "203.0.113.7", meta(hwid), old); err != nil {
			t.Fatal(err)
		}
	}
	if err := st.DeviceUpsert("alice@example.com", "198.51.100.9", meta("away"), old); err != nil {
		t.Fatal(err)
	}

	// Only one IP is online right now (poller stamps the pair).
	if err := st.DeviceUpsert("alice@example.com", "203.0.113.7", store.DeviceMeta{}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	rr := doReq(t, h, http.MethodGet, "/api/devices", c, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rr.Code, rr.Body)
	}
	var rows []map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	online, sameIP := 0, 0
	for _, r := range rows {
		if r["online"] == true {
			online++
			if r["IP"] != "203.0.113.7" {
				t.Fatalf("online row on wrong ip: %v", r["IP"])
			}
		}
		if r["same_ip"] == true {
			sameIP++
			if r["IP"] != "203.0.113.7" {
				t.Fatalf("same_ip row on wrong ip: %v", r["IP"])
			}
		}
	}
	if online != 1 {
		t.Fatalf("online rows = %d, want 1", online)
	}
	if sameIP != 1 {
		t.Fatalf("same_ip rows = %d, want 1", sameIP)
	}
}
