package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"flypanel/internal/store"
)

type fakeTokenStore struct{ tokens map[string]store.APIToken }

func (f fakeTokenStore) APITokenByHash(hash string) (store.APIToken, bool, error) {
	t, ok := f.tokens[hash]
	return t, ok, nil
}

func TestMiddlewareAPIToken(t *testing.T) {
	j := NewJWT(newTestKey(t))
	access, err := j.IssueAccess()
	if err != nil {
		t.Fatalf("IssueAccess(): %v", err)
	}

	valid := store.APIToken{ID: 1, Name: "ci", Scope: "full"}
	read := store.APIToken{ID: 2, Name: "mon", Scope: "read"}
	expired := store.APIToken{ID: 3, Name: "old", Scope: "full",
		ExpiresAt: time.Now().Add(-time.Second)}
	revoked := store.APIToken{ID: 4, Name: "gone", Scope: "full", Revoked: true}

	fake := fakeTokenStore{tokens: map[string]store.APIToken{
		hashToken("fp_valid"):   valid,
		hashToken("fp_read"):    read,
		hashToken("fp_expired"): expired,
		hashToken("fp_revoked"): revoked,
	}}

	cases := []struct {
		name   string
		method string
		path   string
		bearer string
		cookie string
		want   int
		next   bool
	}{
		{"full token allows", http.MethodGet, "/api/stats/summary", "fp_valid", "", 200, true},
		{"read token blocks write", http.MethodPost, "/api/inbounds", "fp_read", "", 403, false},
		{"read token blocks config", http.MethodGet, "/api/config", "fp_read", "", 403, false},
		{"read token blocks routing", http.MethodGet, "/api/config/routing", "fp_read", "", 403, false},
		{"read token blocks users sub", http.MethodGet, "/api/users/7/sub", "fp_read", "", 403, false},
		{"read token blocks inbounds", http.MethodGet, "/api/inbounds", "fp_read", "", 403, false},
		{"read token blocks config subpath", http.MethodGet, "/api/config/outbound", "fp_read", "", 403, false},
		{"read token allows inbounds metrics", http.MethodGet, "/api/inbounds/metrics", "fp_read", "", 200, true},
		{"read token blocks monitoring logs", http.MethodGet, "/api/monitoring/logs", "fp_read", "", 403, false},
		{"read token blocks monitoring caddy", http.MethodGet, "/api/monitoring/caddy", "fp_read", "", 403, false},
		{"read token allows stats", http.MethodGet, "/api/stats/summary", "fp_read", "", 200, true},
		{"expired token", http.MethodGet, "/api/stats/summary", "fp_expired", "", 401, false},
		{"revoked token", http.MethodGet, "/api/stats/summary", "fp_revoked", "", 401, false},
		{"unknown token", http.MethodGet, "/api/stats/summary", "fp_nope", "", 401, false},
		{"bearer on login", http.MethodPost, "/api/auth/login", "fp_valid", "", 401, false},
		{"bearer on token create", http.MethodPost, "/api/tokens", "fp_valid", "", 401, false},
		{"bearer on token delete", http.MethodDelete, "/api/tokens/3", "fp_valid", "", 401, false},
		{"bearer can list tokens", http.MethodGet, "/api/tokens", "fp_valid", "", 401, false},
		{"cookie still works", http.MethodGet, "/api/stats/summary", "", access, 200, true},
		{"cookie works on enroll", http.MethodGet, "/api/auth/2fa/enroll", "", access, 200, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusOK)
			})
			h := Middleware(j, fake)(next)
			req := httptest.NewRequest(tc.method, tc.path, nil)
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: AccessCookie, Value: tc.cookie})
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != tc.want {
				t.Errorf("status = %d, want %d", rr.Code, tc.want)
			}
			if called != tc.next {
				t.Errorf("next called = %v, want %v", called, tc.next)
			}
		})
	}
}
