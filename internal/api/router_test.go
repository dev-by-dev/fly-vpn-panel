package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"flypanel/internal/auth"
	"flypanel/internal/config"
	"flypanel/internal/stats"
	"flypanel/internal/store"
	"flypanel/internal/sub"
	"flypanel/internal/xray"
)

// fakeManager is the test stand-in for *xray.Manager.
type fakeManager struct{ status xray.Status }

func (f *fakeManager) Status() xray.Status { return f.status }
func (f *fakeManager) Start() error        { return nil }
func (f *fakeManager) Stop() error         { return nil }
func (f *fakeManager) Restart() error      { return nil }

// newTestRouter builds a full router over a fresh store. The xray
// client is nil: no test below triggers a membership sync.
func newTestRouter(t *testing.T) (http.Handler, *store.Store, *auth.JWT) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	t.Cleanup(func() { st.Close() })
	j := auth.NewJWT([]byte("0123456789abcdef0123456789abcdef"))
	cfg := &config.Config{
		DataDir: t.TempDir(), EntryIP: "203.0.113.10",
		PanelDomain: "panel.example.com", PollIntervalSec: 30,
	}
	h := NewRouter(Deps{
		Store:   st,
		JWT:     j,
		RL:      auth.NewRateLimiter(5, time.Minute),
		Manager: &fakeManager{status: xray.Status{Running: true, PID: 4242, Restarts: 1, Version: "test-v1"}},
		Cfg:     cfg,
		Poller:  stats.NewPoller(st, nil, nil, 30*time.Second),
	})
	return h, st, j
}

func accessCookie(t *testing.T, j *auth.JWT) *http.Cookie {
	t.Helper()
	tok, err := j.IssueAccess()
	if err != nil {
		t.Fatalf("IssueAccess(): %v", err)
	}
	return &http.Cookie{Name: auth.AccessCookie, Value: tok}
}

func doReq(t *testing.T, h http.Handler, method, path string, c *http.Cookie, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	if c != nil {
		req.AddCookie(c)
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func bodyJSON(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal body %q: %v", rr.Body.String(), err)
	}
	return m
}

func respCookie(t *testing.T, rr *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func createAdmin(t *testing.T, st *store.Store, password string) {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword(): %v", err)
	}
	if err := st.AdminCreate("admin", hash); err != nil {
		t.Fatalf("AdminCreate(): %v", err)
	}
}

func TestRouteAuthMatrix(t *testing.T) {
	h, _, j := newTestRouter(t)
	cookie := accessCookie(t, j)

	cases := []struct {
		name    string
		method  string
		path    string
		cookie  bool
		want    int
		wantErr string
	}{
		{"users without cookie", "GET", "/api/users", false, http.StatusUnauthorized, "unauthorized"},
		{"users with cookie", "GET", "/api/users", true, http.StatusOK, ""},
		{"login without cookie", "POST", "/api/auth/login", false, http.StatusBadRequest, "bad_request"},
		{"2fa enroll without cookie", "POST", "/api/auth/2fa/enroll", false, http.StatusUnauthorized, "unauthorized"},
		{"sub without cookie", "GET", "/sub/xxx", false, http.StatusNotFound, "not found"},
		{"xray status without cookie", "GET", "/api/xray/status", false, http.StatusUnauthorized, "unauthorized"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var c *http.Cookie
			if tc.cookie {
				c = cookie
			}
			rr := doReq(t, h, tc.method, tc.path, c, nil)
			if rr.Code != tc.want {
				t.Fatalf("status = %d (%s), want %d", rr.Code, rr.Body.String(), tc.want)
			}
			if tc.wantErr == "" {
				return
			}
			m := bodyJSON(t, rr)
			if got, _ := m["error"].(string); got != tc.wantErr {
				t.Errorf("error = %q, want %q", got, tc.wantErr)
			}
		})
	}
}

func TestSPAFallback(t *testing.T) {
	h, _, j := newTestRouter(t)

	rr := doReq(t, h, "GET", "/users", nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /users status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("GET /users content-type = %q, want text/html", ct)
	}
	if !bytes.Contains(rr.Body.Bytes(), []byte("build me")) {
		t.Errorf("GET /users body %q missing placeholder", rr.Body.String())
	}

	rr = doReq(t, h, "GET", "/api/nonexistent", accessCookie(t, j), nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("GET /api/nonexistent status = %d, want 404", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("GET /api/nonexistent content-type = %q, want application/json", ct)
	}
	if got, _ := bodyJSON(t, rr)["error"].(string); got != "not found" {
		t.Errorf("error = %q, want not found", got)
	}
}

func TestSettingsWhitelist(t *testing.T) {
	h, st, j := newTestRouter(t)
	cookie := accessCookie(t, j)

	rr := doReq(t, h, "PUT", "/api/settings", cookie, map[string]any{"poll_interval_sec": 60})
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT 60 status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if got, err := st.SettingGet("poll_interval_sec"); err != nil || got != "60" {
		t.Errorf("stored poll_interval_sec = %q, %v; want 60", got, err)
	}

	rr = doReq(t, h, "PUT", "/api/settings", cookie, map[string]any{"unknown_key": "x"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PUT unknown key status = %d (%s), want 400", rr.Code, rr.Body.String())
	}

	rr = doReq(t, h, "PUT", "/api/settings", cookie, map[string]any{"poll_interval_sec": 999})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("PUT 999 status = %d (%s), want 400", rr.Code, rr.Body.String())
	}

	rr = doReq(t, h, "GET", "/api/settings", cookie, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET settings status = %d, want 200", rr.Code)
	}
	m := bodyJSON(t, rr)
	if got, _ := m["poll_interval_sec"].(float64); got != 60 {
		t.Errorf("GET settings poll_interval_sec = %v, want 60", m["poll_interval_sec"])
	}
	if len(m) != 1 {
		t.Errorf("GET settings leaked non-whitelisted keys: %v", m)
	}
}

func TestPasswordChangeRevokesRefresh(t *testing.T) {
	h, st, _ := newTestRouter(t)
	createAdmin(t, st, "old-sufficient-password")

	rr := doReq(t, h, "POST", "/api/auth/login", nil,
		map[string]string{"username": "admin", "password": "old-sufficient-password"})
	if rr.Code != http.StatusOK {
		t.Fatalf("login status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	acc := respCookie(t, rr, "fp_access")
	ref := respCookie(t, rr, "fp_refresh")
	if acc == nil || ref == nil {
		t.Fatal("login did not set both cookies")
	}

	rr = doReq(t, h, "POST", "/api/auth/password", acc,
		map[string]string{"old": "wrong-old-password", "new": "new-sufficient-password"})
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("wrong old status = %d, want 401", rr.Code)
	}

	rr = doReq(t, h, "POST", "/api/auth/password", acc,
		map[string]string{"old": "old-sufficient-password", "new": "short"})
	if rr.Code != http.StatusBadRequest {
		t.Errorf("short new status = %d, want 400", rr.Code)
	}

	rr = doReq(t, h, "POST", "/api/auth/password", acc,
		map[string]string{"old": "old-sufficient-password", "new": "new-sufficient-password"})
	if rr.Code != http.StatusOK {
		t.Fatalf("change status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if ok, _ := bodyJSON(t, rr)["ok"].(bool); !ok {
		t.Error("change body missing ok:true")
	}

	rr = doReq(t, h, "POST", "/api/auth/refresh", ref, nil)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("old refresh after change status = %d, want 401", rr.Code)
	}

	rr = doReq(t, h, "POST", "/api/auth/login", nil,
		map[string]string{"username": "admin", "password": "new-sufficient-password"})
	if rr.Code != http.StatusOK {
		t.Errorf("login with new password status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
}

func TestXrayStatusEndpoint(t *testing.T) {
	h, _, j := newTestRouter(t)

	rr := doReq(t, h, "GET", "/api/xray/status", accessCookie(t, j), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q, want application/json", ct)
	}
	m := bodyJSON(t, rr)
	if got, _ := m["Running"].(bool); !got {
		t.Errorf("Running = %v, want true", m["Running"])
	}
	if got, _ := m["PID"].(float64); got != 4242 {
		t.Errorf("PID = %v, want 4242", m["PID"])
	}
	if got, _ := m["Restarts"].(float64); got != 1 {
		t.Errorf("Restarts = %v, want 1", m["Restarts"])
	}
	if got, _ := m["Version"].(string); got != "test-v1" {
		t.Errorf("Version = %q, want test-v1", got)
	}
	if _, ok := m["Uptime"]; !ok {
		t.Error("Uptime field missing")
	}
}

func TestSeedIdempotent(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	defer st.Close()

	if err := Seed(st); err != nil {
		t.Fatalf("first Seed(): %v", err)
	}
	ins, err := st.InboundList()
	if err != nil {
		t.Fatalf("InboundList(): %v", err)
	}
	if len(ins) != 1 {
		t.Fatalf("inbounds after seed = %d, want 1", len(ins))
	}
	in := ins[0]
	if in.Tag != "vless-xhttp" || !in.Enabled {
		t.Errorf("seeded inbound tag/enabled = %q/%v", in.Tag, in.Enabled)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(in.ConfigJSON), &cfg); err != nil {
		t.Fatalf("seeded config not JSON: %v", err)
	}
	if port, _ := cfg["port"].(float64); port != 8443 {
		t.Errorf("seeded port = %v, want 8443", cfg["port"])
	}
	ss, _ := cfg["streamSettings"].(map[string]any)
	rs, _ := ss["realitySettings"].(map[string]any)
	if dest, _ := rs["dest"].(string); dest != "www.microsoft.com:443" {
		t.Errorf("reality dest = %v, want www.microsoft.com:443", rs["dest"])
	}
	if pbk, _ := rs["publicKey"].(string); len(pbk) != 43 {
		t.Errorf("publicKey length = %d, want 43", len(pbk))
	}
	sids, _ := rs["shortIds"].([]any)
	if len(sids) != 1 {
		t.Fatalf("shortIds = %v, want one", sids)
	}

	jwtKey, err := st.SettingGet("jwt_key")
	if err != nil || jwtKey == "" {
		t.Fatalf("jwt_key after seed = %q, %v", jwtKey, err)
	}
	for _, format := range []string{"json", "mihomo", "clash"} {
		v, err := st.SettingGet("template_" + format)
		if err != nil || v != sub.DefaultTemplate(format) {
			t.Errorf("template_%s = %q, %v; want default", format, v, err)
		}
	}

	if err := Seed(st); err != nil {
		t.Fatalf("second Seed(): %v", err)
	}
	ins, err = st.InboundList()
	if err != nil {
		t.Fatalf("InboundList(): %v", err)
	}
	if len(ins) != 1 {
		t.Fatalf("inbounds after second seed = %d, want 1", len(ins))
	}
	jwtKey2, err := st.SettingGet("jwt_key")
	if err != nil || jwtKey2 != jwtKey {
		t.Errorf("jwt_key changed on reseed: %q → %q (%v)", jwtKey, jwtKey2, err)
	}
}
