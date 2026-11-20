package auth

import (
	"net/http"
)

// Middleware returns an HTTP middleware that requires a valid fp_access
// cookie (verified via j) and otherwise answers 401 {"error":"unauthorized"}.
func Middleware(j *JWT) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c, err := r.Cookie(AccessCookie)
			if err != nil || j.VerifyAccess(c.Value) != nil {
				jsonError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
