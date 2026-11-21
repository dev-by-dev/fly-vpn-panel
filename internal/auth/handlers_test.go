package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"flypanel/internal/store"
)

const testIP = "203.0.113.10:4444"

// newTestEnv opens a store in a temp dir and builds Handlers around it.
func newTestEnv(t *testing.T) *Handlers {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return &Handlers{
		Store: st,
		JWT:   NewJWT(newTestKey(t)),
		RL:    NewRateLimiter(5, time.Minute),
	}
}

func createAdmin(t *testing.T, st *store.Store, password string) {
	t.Helper()
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword(): %v", err)
	}
	if err := st.AdminCreate("admin", hash); err != nil {
		t.Fatalf("AdminCreate(): %v", err)
	}
}

func postJSON(t *testing.T, h http.HandlerFunc, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(b))
	req.RemoteAddr = testIP
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

// postJSONXFF is postJSON with an explicit RemoteAddr and optional
// X-Forwarded-For header (reverse-proxy scenarios).
func postJSONXFF(t *testing.T, h http.HandlerFunc, body any, remoteAddr, xff string) *httptest.ResponseRecorder {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(b))
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rr := httptest.NewRecorder()
	h(rr, req)
	return rr
}

func bodyError(t *testing.T, rr *httptest.ResponseRecorder) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &m); err != nil {
		t.Fatalf("unmarshal body %q: %v", rr.Body.String(), err)
	}
	if e, ok := m["error"].(string); ok {
		return e
	}
	return ""
}

func cookie(rr *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestCookieSecureFlag(t *testing.T) {
	h := newTestEnv(t)
	h.SecureCookies = true
	createAdmin(t, h.Store, "a-sufficiently-long-password")

	rr := postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("login status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	acc := cookie(rr, "fp_access")
	ref := cookie(rr, "fp_refresh")
	if acc == nil || ref == nil {
		t.Fatal("auth cookies missing after login")
	}
	if !acc.Secure || !ref.Secure {
		t.Error("Secure flag missing on issued cookies (SecureCookies=true)")
	}

	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: RefreshCookie, Value: ref.Value})
	rr = httptest.NewRecorder()
	h.Refresh(rr, req)
	if got := cookie(rr, "fp_access"); got == nil || !got.Secure {
		t.Error("Secure flag missing on refresh-issued access cookie")
	}

	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.RemoteAddr = testIP
	rr = httptest.NewRecorder()
	h.Logout(rr, req)
	for _, name := range []string{"fp_access", "fp_refresh"} {
		c := cookie(rr, name)
		if c == nil || c.MaxAge >= 0 {
			t.Fatalf("logout did not clear %s", name)
		}
		if !c.Secure {
			t.Errorf("Secure flag missing on cleared %s cookie", name)
		}
	}

	h2 := newTestEnv(t)
	createAdmin(t, h2.Store, "a-sufficiently-long-password")
	rr = postJSON(t, h2.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password",
	})
	if got := cookie(rr, "fp_access"); got == nil || got.Secure {
		t.Error("Secure flag set on issued cookie (SecureCookies=false)")
	}
}

func TestLoginFlow(t *testing.T) {
	h := newTestEnv(t)
	createAdmin(t, h.Store, "a-sufficiently-long-password")

	rr := postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password",
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("login status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if bodyError(t, rr) != "" {
		t.Errorf("login body error = %q, want none", bodyError(t, rr))
	}
	acc := cookie(rr, "fp_access")
	ref := cookie(rr, "fp_refresh")
	if acc == nil || acc.Value == "" {
		t.Error("fp_access cookie missing after login")
	}
	if ref == nil || ref.Value == "" {
		t.Error("fp_refresh cookie missing after login")
	}
	if ref.Path != "/api/auth" {
		t.Errorf("fp_refresh path = %q, want /api/auth", ref.Path)
	}
	if !acc.HttpOnly || !ref.HttpOnly {
		t.Error("auth cookies must be HttpOnly")
	}
	if acc.SameSite != http.SameSiteStrictMode || ref.SameSite != http.SameSiteStrictMode {
		t.Error("auth cookies must be SameSite=Strict")
	}
	if err := h.JWT.VerifyAccess(acc.Value); err != nil {
		t.Errorf("issued access token invalid: %v", err)
	}

	rr = postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "wrong-password-xxx",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password status = %d, want 401", rr.Code)
	}
	if got := bodyError(t, rr); got != "invalid_credentials" {
		t.Errorf("wrong-password error = %q, want invalid_credentials", got)
	}
}

func TestLogin2FARequired(t *testing.T) {
	h := newTestEnv(t)
	createAdmin(t, h.Store, "a-sufficiently-long-password")
	secret, _, err := EnrollTOTP("admin")
	if err != nil {
		t.Fatalf("EnrollTOTP(): %v", err)
	}
	if err := h.Store.AdminSetTOTP(secret, true); err != nil {
		t.Fatalf("AdminSetTOTP(): %v", err)
	}

	// No code -> 2fa_required.
	rr := postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password",
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no-code status = %d, want 401", rr.Code)
	}
	if got := bodyError(t, rr); got != "2fa_required" {
		t.Errorf("no-code error = %q, want 2fa_required", got)
	}

	// Wrong code -> 2fa_required.
	rr = postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password", "code": "000000",
	})
	if rr.Code != http.StatusUnauthorized || bodyError(t, rr) != "2fa_required" {
		t.Fatalf("wrong-code = %d %q, want 401 2fa_required", rr.Code, bodyError(t, rr))
	}

	// Valid TOTP code -> ok.
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode(): %v", err)
	}
	rr = postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password", "code": code,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("totp-code status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if cookie(rr, "fp_access") == nil {
		t.Error("fp_access cookie missing after 2FA login")
	}

	// Backup code -> ok once, then rejected (consumed).
	codes, hashes, err := GenerateBackupCodes()
	if err != nil {
		t.Fatalf("GenerateBackupCodes(): %v", err)
	}
	if err := h.Store.BackupCodesReplace(hashes); err != nil {
		t.Fatalf("BackupCodesReplace(): %v", err)
	}
	rr = postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password", "code": codes[0],
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("backup-code status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	// Same backup code again -> 401 (single use).
	rr = postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password", "code": codes[0],
	})
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("backup-code reuse status = %d, want 401", rr.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	h := newTestEnv(t)
	createAdmin(t, h.Store, "a-sufficiently-long-password")
	for i := 0; i < 5; i++ {
		rr := postJSON(t, h.Login, map[string]string{
			"username": "admin", "password": "wrong-password-xxx",
		})
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt #%d status = %d, want 401", i+1, rr.Code)
		}
	}
	rr := postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password",
	})
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("6th request status = %d, want 429", rr.Code)
	}
}

// TestLoginRateLimitXFF: behind the reverse proxy every client shares
// RemoteAddr 127.0.0.1; the limiter must key on the first
// X-Forwarded-For IP (loopback only) so clients get independent
// budgets instead of one shared global 5/min limit.
func TestLoginRateLimitXFF(t *testing.T) {
	h := newTestEnv(t)
	createAdmin(t, h.Store, "a-sufficiently-long-password")
	const loopback = "127.0.0.1:5555"
	creds := func() map[string]string {
		return map[string]string{"username": "admin", "password": "wrong-password-xxx"}
	}

	for i := 0; i < 5; i++ {
		rr := postJSONXFF(t, h.Login, creds(), loopback, "203.0.113.20")
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt #%d status = %d, want 401", i+1, rr.Code)
		}
	}
	// Same X-FF: that client's budget is exhausted.
	if rr := postJSONXFF(t, h.Login, creds(), loopback, "203.0.113.20"); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("6th same-XFF status = %d, want 429", rr.Code)
	}
	// A different X-FF client has an independent budget: 401, not 429.
	if rr := postJSONXFF(t, h.Login, creds(), loopback, "198.51.100.30"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("different XFF status = %d, want 401 (independent rate limit)", rr.Code)
	}
	// Off-loopback RemoteAddr must ignore XFF entirely: this fresh
	// client reuses the exhausted XFF value and must not be 429'd by it.
	if rr := postJSONXFF(t, h.Login, creds(), "203.0.113.99:6666", "203.0.113.20"); rr.Code != http.StatusUnauthorized {
		t.Fatalf("direct client status = %d, want 401 (XFF ignored off-loopback)", rr.Code)
	}
}

// TestEnroll2FAAlreadyEnabled: re-running enroll while 2FA is on calls
// AdminSetTOTP(secret,false) and would silently disable 2FA without a
// code; it must 409 and require disable-with-code first.
func TestEnroll2FAAlreadyEnabled(t *testing.T) {
	h := newTestEnv(t)
	createAdmin(t, h.Store, "a-sufficiently-long-password")
	secret, _, err := EnrollTOTP("admin")
	if err != nil {
		t.Fatalf("EnrollTOTP(): %v", err)
	}
	if err := h.Store.AdminSetTOTP(secret, true); err != nil {
		t.Fatalf("AdminSetTOTP(): %v", err)
	}

	rr := postJSON(t, h.Enroll2FA, map[string]string{})
	if rr.Code != http.StatusConflict {
		t.Fatalf("enroll-while-enabled status = %d (%s), want 409", rr.Code, rr.Body.String())
	}
	// 2FA must still be enabled and the secret untouched afterwards.
	admin, err := h.Store.AdminGet()
	if err != nil {
		t.Fatal(err)
	}
	if !admin.TOTPEnabled || admin.TOTPSecret != secret {
		t.Fatalf("admin after rejected enroll: enabled=%v secret=%q, want untouched",
			admin.TOTPEnabled, admin.TOTPSecret)
	}
}

func TestRefreshLogout(t *testing.T) {
	h := newTestEnv(t)
	createAdmin(t, h.Store, "a-sufficiently-long-password")

	rr := postJSON(t, h.Login, map[string]string{
		"username": "admin", "password": "a-sufficiently-long-password",
	})
	ref := cookie(rr, "fp_refresh")
	if ref == nil {
		t.Fatal("no fp_refresh cookie after login")
	}

	// Refresh issues a new access cookie.
	req := httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.RemoteAddr = testIP
	req.AddCookie(&http.Cookie{Name: "fp_refresh", Value: ref.Value})
	rr = httptest.NewRecorder()
	h.Refresh(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("refresh status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if cookie(rr, "fp_access") == nil {
		t.Error("fp_access cookie missing after refresh")
	}

	// Logout revokes the refresh token.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	req.RemoteAddr = testIP
	req.AddCookie(&http.Cookie{Name: "fp_refresh", Value: ref.Value})
	rr = httptest.NewRecorder()
	h.Logout(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", rr.Code)
	}

	// The old refresh token no longer grants access.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.RemoteAddr = testIP
	req.AddCookie(&http.Cookie{Name: "fp_refresh", Value: ref.Value})
	rr = httptest.NewRecorder()
	h.Refresh(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout status = %d, want 401", rr.Code)
	}

	// Missing refresh cookie -> 401.
	req = httptest.NewRequest(http.MethodPost, "/api/auth/refresh", nil)
	req.RemoteAddr = testIP
	rr = httptest.NewRecorder()
	h.Refresh(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("refresh without cookie status = %d, want 401", rr.Code)
	}
}

func TestMiddleware(t *testing.T) {
	j := NewJWT(newTestKey(t))
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := Middleware(j)(next)

	do := func(token string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/anything", nil)
		if token != "" {
			req.AddCookie(&http.Cookie{Name: "fp_access", Value: token})
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}

	if code := do(""); code != http.StatusUnauthorized {
		t.Errorf("no cookie status = %d, want 401", code)
	}
	tok, err := j.IssueAccess()
	if err != nil {
		t.Fatalf("IssueAccess(): %v", err)
	}
	if code := do(tok); code != http.StatusOK {
		t.Errorf("valid token status = %d, want 200", code)
	}
	if code := do(tok + "x"); code != http.StatusUnauthorized {
		t.Errorf("tampered token status = %d, want 401", code)
	}
}

// TestVerify2FADoubleKeepsCodes guards against an HTTP retry / double-submit
// of Verify2FA wiping the already-installed backup codes.
func TestVerify2FADoubleKeepsCodes(t *testing.T) {
	h := newTestEnv(t)
	createAdmin(t, h.Store, "a-sufficiently-long-password")

	rr := postJSON(t, h.Enroll2FA, map[string]string{})
	if rr.Code != http.StatusOK {
		t.Fatalf("enroll status = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	var enrollResp struct {
		Secret      string   `json:"secret"`
		BackupCodes []string `json:"backup_codes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &enrollResp); err != nil {
		t.Fatalf("unmarshal enroll body: %v", err)
	}

	code, err := totp.GenerateCode(enrollResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode(): %v", err)
	}
	rr = postJSON(t, h.Verify2FA, map[string]string{"code": code})
	if rr.Code != http.StatusOK {
		t.Fatalf("first verify status = %d (%s), want 200", rr.Code, rr.Body.String())
	}

	// Second Verify2FA with a fresh valid TOTP code (retry) must succeed
	// and must NOT wipe the installed backup codes.
	code, err = totp.GenerateCode(enrollResp.Secret, time.Now())
	if err != nil {
		t.Fatalf("GenerateCode(): %v", err)
	}
	rr = postJSON(t, h.Verify2FA, map[string]string{"code": code})
	if rr.Code != http.StatusOK {
		t.Fatalf("second verify status = %d (%s), want 200", rr.Code, rr.Body.String())
	}

	ok, err := h.Store.BackupCodeConsume(backupHash(enrollResp.BackupCodes[0]))
	if err != nil {
		t.Fatalf("BackupCodeConsume(): %v", err)
	}
	if !ok {
		t.Fatal("backup code 0 not consumable after double Verify2FA — codes were wiped")
	}
	ok, err = h.Store.BackupCodeConsume(backupHash(enrollResp.BackupCodes[0]))
	if err != nil {
		t.Fatalf("BackupCodeConsume() reuse: %v", err)
	}
	if ok {
		t.Fatal("backup code 0 consumed twice, want single-use")
	}
}

func TestAdminInitSecondFails(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("store.Open(): %v", err)
	}
	defer st.Close()

	if err := AdminInit(st, "admin", "long-enough-password-1"); err != nil {
		t.Fatalf("first AdminInit(): %v", err)
	}
	if err := AdminInit(st, "admin", "long-enough-password-2"); err == nil {
		t.Fatal("second AdminInit() = nil, want error (admin exists)")
	}

	// Password policy: shorter than 12 chars is rejected.
	if err := AdminInit(st, "admin", "short"); err == nil {
		t.Fatal("AdminInit(short password) = nil, want error")
	}
}
