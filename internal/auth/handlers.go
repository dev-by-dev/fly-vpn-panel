package auth

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"flypanel/internal/store"
)

// pendingBackupKey holds the backup-code hashes generated at enrollment
// until Verify2FA promotes them into backup_codes (codes are shown once).
const pendingBackupKey = "2fa_pending_hashes"

// Handlers holds the auth HTTP handler dependencies. The 2FA endpoints are
// plain handlers; the router (Task 10) mounts them behind the JWT middleware.
type Handlers struct {
	Store *store.Store
	JWT   *JWT
	RL    *RateLimiter
}

// --- request/response shapes ---

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Code     string `json:"code"`
}

type codeRequest struct {
	Code string `json:"code"`
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(v)
}

// --- cookies ---

func accessCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     AccessCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   int(AccessTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

func refreshCookie(value string) *http.Cookie {
	return &http.Cookie{
		Name:     RefreshCookie,
		Value:    value,
		Path:     "/api/auth",
		MaxAge:   int(RefreshTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	}
}

func setAuthCookies(w http.ResponseWriter, access, refresh string) {
	http.SetCookie(w, accessCookie(access))
	http.SetCookie(w, refreshCookie(refresh))
}

func clearAuthCookies(w http.ResponseWriter) {
	del := &http.Cookie{Name: AccessCookie, Value: "", Path: "/", MaxAge: -1}
	http.SetCookie(w, del)
	del = &http.Cookie{Name: RefreshCookie, Value: "", Path: "/api/auth", MaxAge: -1}
	http.SetCookie(w, del)
}

// --- handlers ---

// Login authenticates the admin: rate limit by IP, credentials check, then
// (if 2FA is enabled) a TOTP or single-use backup code. On success it sets
// the fp_access and fp_refresh cookies.
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if !h.RL.Allow(ip) {
		jsonError(w, http.StatusTooManyRequests, "rate_limited")
		return
	}

	var req loginRequest
	if err := decodeJSON(r, &req); err != nil || req.Username == "" || req.Password == "" {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}

	admin, err := h.Store.AdminGet()
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if admin.Username != req.Username || !VerifyPassword(admin.PasswordHash, req.Password) {
		jsonError(w, http.StatusUnauthorized, "invalid_credentials")
		return
	}

	if admin.TOTPEnabled {
		code := normalizeCode(req.Code)
		if code == "" {
			jsonError(w, http.StatusUnauthorized, "2fa_required")
			return
		}
		if !ValidateTOTP(admin.TOTPSecret, code) {
			ok, err := h.Store.BackupCodeConsume(backupHash(code))
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "internal")
				return
			}
			if !ok {
				jsonError(w, http.StatusUnauthorized, "2fa_required")
				return
			}
		}
	}

	if err := h.issueSession(w); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// issueSession mints the access and refresh tokens, persists the refresh
// hash and drops both cookies.
func (h *Handlers) issueSession(w http.ResponseWriter) error {
	access, err := h.JWT.IssueAccess()
	if err != nil {
		return err
	}
	raw, hash, err := h.JWT.IssueRefresh()
	if err != nil {
		return err
	}
	if err := h.Store.RefreshTokenInsert(hash, time.Now().Add(RefreshTTL)); err != nil {
		return err
	}
	setAuthCookies(w, access, raw)
	return nil
}

// Refresh exchanges a valid fp_refresh cookie for a new fp_access cookie.
// Refresh tokens are not rotated in v1.
func (h *Handlers) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(RefreshCookie)
	if err != nil {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	valid, err := h.Store.RefreshTokenValid(hashToken(c.Value))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if !valid {
		jsonError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	access, err := h.JWT.IssueAccess()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	http.SetCookie(w, accessCookie(access))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Logout revokes the refresh token (when presented) and clears the cookies.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(RefreshCookie); err == nil {
		// Unknown hashes are not an error for logout.
		_ = h.Store.RefreshTokenRevoke(hashToken(c.Value))
	}
	clearAuthCookies(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Enroll2FA generates a TOTP secret, otpauth URI and 10 backup codes, saves
// the secret with totp_enabled still off, and returns the codes exactly once.
// The code hashes wait in settings until Verify2FA promotes them.
func (h *Handlers) Enroll2FA(w http.ResponseWriter, r *http.Request) {
	admin, err := h.Store.AdminGet()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	secret, uri, err := EnrollTOTP(admin.Username)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	codes, hashes, err := GenerateBackupCodes()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.Store.AdminSetTOTP(secret, false); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.Store.SettingSet(pendingBackupKey, strings.Join(hashes, ",")); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"secret":       secret,
		"otpauth_uri":  uri,
		"backup_codes": codes,
	})
}

// Verify2FA confirms enrollment: a valid TOTP code enables 2FA and installs
// the backup codes generated by Enroll2FA.
func (h *Handlers) Verify2FA(w http.ResponseWriter, r *http.Request) {
	admin, err := h.Store.AdminGet()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	var req codeRequest
	if err := decodeJSON(r, &req); err != nil || req.Code == "" {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if !ValidateTOTP(admin.TOTPSecret, normalizeCode(req.Code)) {
		jsonError(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	pending, err := h.Store.SettingGet(pendingBackupKey)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	hashes := []string{}
	if pending != "" {
		hashes = strings.Split(pending, ",")
	}
	if err := h.Store.BackupCodesReplace(hashes); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.Store.AdminSetTOTP(admin.TOTPSecret, true); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.Store.SettingSet(pendingBackupKey, ""); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Disable2FA turns 2FA off and clears the stored secret when the current
// TOTP code is valid.
func (h *Handlers) Disable2FA(w http.ResponseWriter, r *http.Request) {
	admin, err := h.Store.AdminGet()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	var req codeRequest
	if err := decodeJSON(r, &req); err != nil || req.Code == "" {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if admin.TOTPSecret == "" || !ValidateTOTP(admin.TOTPSecret, normalizeCode(req.Code)) {
		jsonError(w, http.StatusUnauthorized, "invalid_code")
		return
	}
	if err := h.Store.AdminSetTOTP("", false); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// normalizeCode trims and upper-cases a submitted 2FA code (backup codes are
// base32; TOTP codes are digits).
func normalizeCode(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// AdminInit creates the single admin account with an argon2id hash. It is
// the store-level half of `flypanel admin init`; the CLI half lives in
// cmd/flypanel.
func AdminInit(st *store.Store, username, password string) error {
	if username == "" {
		return errors.New("username must not be empty")
	}
	if len(password) < 12 {
		return errors.New("password must be at least 12 characters")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	return st.AdminCreate(username, hash)
}
