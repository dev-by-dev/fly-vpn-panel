package auth

import (
	"net/http"
	"strings"
	"time"

	"flypanel/internal/store"
)

// APITokenStore looks up a long-lived API token by its SHA-256 hash.
type APITokenStore interface {
	APITokenByHash(hash string) (store.APIToken, bool, error)
}

// Middleware requires either a valid fp_access cookie or, outside the
// cookie-only paths, a Bearer API token. Read-scope tokens may only perform
// GET/HEAD and never reach config or user endpoints.
func Middleware(j *JWT, tokens APITokenStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if c, err := r.Cookie(AccessCookie); err == nil && j.VerifyAccess(c.Value) == nil {
				next.ServeHTTP(w, r)
				return
			}

			raw, ok := bearerToken(r)
			if !ok || tokens == nil {
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			// Tokens must not mint tokens or touch the auth surface.
			if cookieOnly(r) {
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}

			t, found, err := tokens.APITokenByHash(hashToken(raw))
			if err != nil || !found || t.Revoked {
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if !t.ExpiresAt.IsZero() && !t.ExpiresAt.After(time.Now()) {
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			if t.Scope == "read" {
				if r.Method != http.MethodGet && r.Method != http.MethodHead {
					jsonError(w, http.StatusForbidden, "forbidden")
					return
				}
				if readDenied(r.URL.Path) {
					jsonError(w, http.StatusForbidden, "forbidden")
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if len(h) <= len(prefix) || !strings.EqualFold(h[:len(prefix)], prefix) {
		return "", false
	}
	return strings.TrimSpace(h[len(prefix):]), true
}

// cookieOnly reports paths where only a browser session may act.
func cookieOnly(r *http.Request) bool {
	if strings.HasPrefix(r.URL.Path, "/api/auth/") {
		return true
	}
	return strings.HasPrefix(r.URL.Path, "/api/tokens")
}

// readDenied hides secrets a monitoring token has no business seeing.
func readDenied(path string) bool {
	if path == "/api/inbounds/metrics" {
		return false
	}
	if strings.HasPrefix(path, "/api/inbounds") || strings.HasPrefix(path, "/api/config") || strings.HasPrefix(path, "/api/monitoring") {
		return true
	}
	return path == "/api/users" || strings.HasPrefix(path, "/api/users/")
}
