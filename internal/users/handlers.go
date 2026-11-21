package users

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"flypanel/internal/store"
)

// Handlers serves the /api/users endpoints. Client is the xray
// membership surface; membership changes are hot-synced immediately via
// SyncMembership instead of waiting for the poller. PollInterval drives
// the online_count freshness window (devices seen within two
// intervals); Task 10 wires it from config.
type Handlers struct {
	Store        *store.Store
	Client       MemberSync
	PollInterval time.Duration
}

type userResponse struct {
	ID             int64     `json:"id"`
	Username       string    `json:"username"`
	UUID           string    `json:"uuid"`
	SubToken       string    `json:"sub_token"`
	Expiry         time.Time `json:"expiry"`
	QuotaBytes     int64     `json:"quota_bytes"`
	QuotaPeriod    string    `json:"quota_period"`
	QuotaResetAt   time.Time `json:"quota_reset_at"`
	DeviceLimit    int       `json:"device_limit"`
	Disabled       bool      `json:"disabled"`
	DisabledReason string    `json:"disabled_reason"`
	CreatedAt      time.Time `json:"created_at"`
	UsedBytes      int64     `json:"used_bytes"`
	OnlineCount    int       `json:"online_count"`
}

type createRequest struct {
	Username    string `json:"username"`
	Expiry      string `json:"expiry"`
	QuotaBytes  int64  `json:"quota_bytes"`
	QuotaPeriod string `json:"quota_period"`
	DeviceLimit int    `json:"device_limit"`
}

type patchRequest struct {
	Username    *string `json:"username"`
	Expiry      *string `json:"expiry"`
	QuotaBytes  *int64  `json:"quota_bytes"`
	QuotaPeriod *string `json:"quota_period"`
	DeviceLimit *int    `json:"device_limit"`
	Disabled    *bool   `json:"disabled"`
}

var validPeriods = map[string]bool{"none": true, "day": true, "week": true, "month": true}

// validUsername: usernames are the per-user key in xray's stats service
// (OnlineUsers/QueryUserStats); anything outside [A-Za-z0-9._@-] breaks
// counter extraction, so it is rejected at the boundary.
func validUsername(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == '@' || r == '-':
		default:
			return false
		}
	}
	return true
}

const (
	tokenAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	tokenLen      = 16
)

// newSubToken returns a 16-char base62 token from crypto/rand. Bytes are
// rejection-sampled below 248 (62*4) to avoid modulo bias.
func newSubToken() (string, error) {
	out := make([]byte, tokenLen)
	for i := 0; i < tokenLen; {
		var b [1]byte
		if _, err := rand.Read(b[:]); err != nil {
			return "", err
		}
		if b[0] >= 248 {
			continue
		}
		out[i] = tokenAlphabet[b[0]%62]
		i++
	}
	return string(out), nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func decodeJSON(r *http.Request, v any) error {
	return json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(v)
}

func userID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

// enrich marshals u with used_bytes (samples since quota_reset_at) and
// online_count (devices seen within two poll intervals) for the UI table.
func (h *Handlers) enrich(u *store.User, now time.Time) (userResponse, error) {
	up, down, err := h.Store.SamplesSince(u.Username, u.QuotaResetAt)
	if err != nil {
		return userResponse{}, err
	}
	devices, err := h.Store.DevicesForUser(u.Username)
	if err != nil {
		return userResponse{}, err
	}
	cutoff := now.Add(-2 * h.PollInterval)
	online := 0
	for _, d := range devices {
		if !d.LastSeen.Before(cutoff) {
			online++
		}
	}
	return userResponse{
		ID: u.ID, Username: u.Username, UUID: u.UUID, SubToken: u.SubToken,
		Expiry: u.Expiry, QuotaBytes: u.QuotaBytes, QuotaPeriod: u.QuotaPeriod,
		QuotaResetAt: u.QuotaResetAt, DeviceLimit: u.DeviceLimit, Disabled: u.Disabled,
		DisabledReason: u.DisabledReason, CreatedAt: u.CreatedAt,
		UsedBytes: up + down, OnlineCount: online,
	}, nil
}

// hotSync reconciles xray membership with the store right away.
func (h *Handlers) hotSync(ctx context.Context) error {
	users, err := h.Store.UserList()
	if err != nil {
		return err
	}
	inbounds, err := h.Store.InboundListEnabled()
	if err != nil {
		return err
	}
	return SyncMembership(ctx, h.Client, inbounds, users)
}

func (h *Handlers) respondUser(w http.ResponseWriter, status int, u *store.User) {
	resp, err := h.enrich(u, time.Now().UTC())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, status, resp)
}

// List serves GET /api/users.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.Store.UserList()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	now := time.Now().UTC()
	out := make([]userResponse, 0, len(users))
	for i := range users {
		resp, err := h.enrich(&users[i], now)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "internal")
			return
		}
		out = append(out, resp)
	}
	writeJSON(w, http.StatusOK, out)
}

// Get serves GET /api/users/{id}.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, err := userID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	u, err := h.Store.UserGet(id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	h.respondUser(w, http.StatusOK, u)
}

// Create serves POST /api/users: uuid v4, 16-char base62 sub_token,
// quota_reset_at at the current period start. expiry is required.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.Username == "" {
		jsonError(w, http.StatusBadRequest, "username_required")
		return
	}
	if !validUsername(req.Username) {
		jsonError(w, http.StatusBadRequest, "invalid_username")
		return
	}
	if req.QuotaPeriod == "" {
		req.QuotaPeriod = "none" // store schema default
	}
	if !validPeriods[req.QuotaPeriod] {
		jsonError(w, http.StatusBadRequest, "invalid_quota_period")
		return
	}
	if req.QuotaBytes < 0 || req.DeviceLimit < 0 {
		jsonError(w, http.StatusBadRequest, "negative_value")
		return
	}
	expiry, err := time.Parse(time.RFC3339, req.Expiry)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_expiry")
		return
	}
	id, err := uuid.NewRandom()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	token, err := newSubToken()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	now := time.Now().UTC()
	u := &store.User{
		Username:     req.Username,
		UUID:         id.String(),
		SubToken:     token,
		Expiry:       expiry.UTC(),
		QuotaBytes:   req.QuotaBytes,
		QuotaPeriod:  req.QuotaPeriod,
		QuotaResetAt: periodStart(now, req.QuotaPeriod),
		DeviceLimit:  req.DeviceLimit,
		CreatedAt:    now,
	}
	if err := h.Store.UserCreate(u); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonError(w, http.StatusBadRequest, "duplicate_username")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	// The row is committed; if xray is unreachable the poller's reconcile
	// heals it, but the failure is surfaced so the admin knows.
	if err := h.hotSync(r.Context()); err != nil {
		jsonError(w, http.StatusBadGateway, "sync_failed")
		return
	}
	h.respondUser(w, http.StatusCreated, u)
}

// Patch serves PATCH /api/users/{id}. username is immutable (it is the
// stats key). Changing quota_period restarts the period at the current
// period start; raising quota_bytes re-enables a quota-disabled user.
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := userID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	var req patchRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.Username != nil {
		jsonError(w, http.StatusBadRequest, "username_immutable")
		return
	}
	u, err := h.Store.UserGet(id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}

	oldQuota := u.QuotaBytes
	if req.QuotaPeriod != nil {
		if *req.QuotaPeriod == "" {
			*req.QuotaPeriod = "none"
		}
		if !validPeriods[*req.QuotaPeriod] {
			jsonError(w, http.StatusBadRequest, "invalid_quota_period")
			return
		}
		if *req.QuotaPeriod != u.QuotaPeriod {
			u.QuotaPeriod = *req.QuotaPeriod
			u.QuotaResetAt = periodStart(time.Now().UTC(), u.QuotaPeriod)
		}
	}
	if req.QuotaBytes != nil {
		if *req.QuotaBytes < 0 {
			jsonError(w, http.StatusBadRequest, "negative_value")
			return
		}
		u.QuotaBytes = *req.QuotaBytes
	}
	if req.DeviceLimit != nil {
		if *req.DeviceLimit < 0 {
			jsonError(w, http.StatusBadRequest, "negative_value")
			return
		}
		u.DeviceLimit = *req.DeviceLimit
	}
	if req.Expiry != nil {
		expiry, err := time.Parse(time.RFC3339, *req.Expiry)
		if err != nil {
			jsonError(w, http.StatusBadRequest, "invalid_expiry")
			return
		}
		u.Expiry = expiry.UTC()
	}
	if req.Disabled != nil {
		u.Disabled = *req.Disabled
		if u.Disabled {
			u.DisabledReason = "manual"
		} else {
			u.DisabledReason = ""
		}
	}
	// Raising the quota re-enables a user disabled for exceeding it.
	if u.Disabled && u.DisabledReason == "quota" && u.QuotaBytes > oldQuota {
		u.Disabled = false
		u.DisabledReason = ""
	}

	if err := h.Store.UserUpdate(u); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.hotSync(r.Context()); err != nil {
		jsonError(w, http.StatusBadGateway, "sync_failed")
		return
	}
	h.respondUser(w, http.StatusOK, u)
}

// Delete serves DELETE /api/users/{id}: the user is removed from every
// enabled inbound before the row is deleted, then membership is
// reconciled for the remaining users.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := userID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	u, err := h.Store.UserGet(id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	inbounds, err := h.Store.InboundListEnabled()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	var errs []error
	for _, in := range inbounds {
		if err := h.Client.RemoveUser(in.Tag, u.Username); err != nil {
			errs = append(errs, fmt.Errorf("remove %q on %q: %w", u.Username, in.Tag, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		jsonError(w, http.StatusBadGateway, "sync_failed")
		return
	}
	if err := h.Store.UserDelete(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.hotSync(r.Context()); err != nil {
		jsonError(w, http.StatusBadGateway, "sync_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// ResetQuota serves POST /api/users/{id}/reset-quota: the quota window
// restarts at the current period start (usage sums samples from
// quota_reset_at, so older usage stops counting), and a quota-disabled
// user is re-enabled. For period none there is no period start, so the
// window restarts at "now" — the zero time would keep counting
// lifetime usage and the poller would re-disable on the next tick.
func (h *Handlers) ResetQuota(w http.ResponseWriter, r *http.Request) {
	id, err := userID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	u, err := h.Store.UserGet(id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if u.QuotaPeriod == "none" {
		u.QuotaResetAt = time.Now().UTC()
	} else {
		u.QuotaResetAt = periodStart(time.Now().UTC(), u.QuotaPeriod)
	}
	if u.Disabled && u.DisabledReason == "quota" {
		u.Disabled = false
		u.DisabledReason = ""
	}
	if err := h.Store.UserUpdate(u); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.hotSync(r.Context()); err != nil {
		jsonError(w, http.StatusBadGateway, "sync_failed")
		return
	}
	h.respondUser(w, http.StatusOK, u)
}
