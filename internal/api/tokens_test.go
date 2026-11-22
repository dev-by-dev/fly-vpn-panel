package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doBearer(t *testing.T, h http.Handler, method, path, raw string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("Authorization", "Bearer "+raw)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestAPITokens(t *testing.T) {
	h, st, j := newTestRouter(t)
	createAdmin(t, st, "correct horse battery")
	cookie := accessCookie(t, j)

	rr := doReq(t, h, http.MethodPost, "/api/tokens", cookie, map[string]any{
		"name": "ci", "scope": "read", "expires_in_days": 30,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d (%s)", rr.Code, rr.Body.String())
	}
	created := bodyJSON(t, rr)
	raw, _ := created["token"].(string)
	if !strings.HasPrefix(raw, "fp_") {
		t.Fatalf("token = %q, want fp_ prefix", raw)
	}
	if created["scope"] != "read" || created["expires_at"] == "" {
		t.Fatalf("unexpected create body: %v", created)
	}

	rr = doReq(t, h, http.MethodGet, "/api/tokens", cookie, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d", rr.Code)
	}
	tokens, _ := bodyJSON(t, rr)["tokens"].([]any)
	if len(tokens) != 1 {
		t.Fatalf("list len = %d, want 1 (%s)", len(tokens), rr.Body.String())
	}
	if row, _ := tokens[0].(map[string]any); row["token"] != nil {
		t.Fatalf("list leaked the raw token: %v", row)
	}

	// The minted token authenticates on its own.
	if code := doBearer(t, h, http.MethodGet, "/api/xray/status", raw).Code; code != http.StatusOK {
		t.Errorf("bearer GET status = %d, want 200", code)
	}
	// Read scope cannot write or reach config/users.
	if code := doBearer(t, h, http.MethodPost, "/api/xray/restart", raw).Code; code != http.StatusForbidden {
		t.Errorf("read POST status = %d, want 403", code)
	}
	if code := doBearer(t, h, http.MethodGet, "/api/config", raw).Code; code != http.StatusForbidden {
		t.Errorf("read config status = %d, want 403", code)
	}
	// Tokens cannot mint tokens.
	if code := doBearer(t, h, http.MethodPost, "/api/tokens", raw).Code; code != http.StatusUnauthorized {
		t.Errorf("bearer create status = %d, want 401", code)
	}

	// Revoke kills the token immediately.
	if code := doReq(t, h, http.MethodDelete, "/api/tokens/1", cookie, nil).Code; code != http.StatusNoContent {
		t.Fatalf("revoke status = %d", code)
	}
	if code := doBearer(t, h, http.MethodGet, "/api/xray/status", raw).Code; code != http.StatusUnauthorized {
		t.Errorf("revoked token status = %d, want 401", code)
	}
	if code := doReq(t, h, http.MethodDelete, "/api/tokens/1", cookie, nil).Code; code != http.StatusNotFound {
		t.Errorf("second revoke status = %d, want 404", code)
	}
}

func TestAPITokenValidation(t *testing.T) {
	h, st, j := newTestRouter(t)
	createAdmin(t, st, "correct horse battery")
	cookie := accessCookie(t, j)

	bodies := []map[string]any{
		{"name": "  ", "scope": "read", "expires_in_days": 30},
		{"name": "ci", "scope": "admin", "expires_in_days": 30},
		{"name": "ci", "scope": "read", "expires_in_days": 7},
		{"name": strings.Repeat("x", 65), "scope": "read", "expires_in_days": 30},
	}
	for _, body := range bodies {
		rr := doReq(t, h, http.MethodPost, "/api/tokens", cookie, body)
		if rr.Code != http.StatusBadRequest {
			t.Errorf("body %v status = %d, want 400", body, rr.Code)
		}
	}
	// Never-expiring token is allowed and reports an empty expiry.
	rr := doReq(t, h, http.MethodPost, "/api/tokens", cookie, map[string]any{
		"name": "ops", "scope": "full", "expires_in_days": 0,
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("never token status = %d (%s)", rr.Code, rr.Body.String())
	}
	if bodyJSON(t, rr)["expires_at"] != "" {
		t.Errorf("never token expires_at = %v, want \"\"", bodyJSON(t, rr)["expires_at"])
	}
}
