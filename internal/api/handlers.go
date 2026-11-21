package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"flypanel/internal/config"
	"flypanel/internal/stats"
	"flypanel/internal/store"
	"flypanel/internal/sub"
	"flypanel/internal/xray"
)

// Settings keys owned by the API layer.
const (
	jwtKeySetting   = "jwt_key"
	pollIntervalKey = "poll_interval_sec"
	pollIntervalMin = 5
	pollIntervalMax = 300
	defaultPollSec  = 30
)

// Handlers serves the /api/settings, /api/xray, /api/stats and
// /api/users/{id}/sub endpoints — the surfaces without a domain
// package of their own.
type Handlers struct {
	Store       *store.Store
	Mgr         Manager
	Poller      *stats.Poller
	PanelDomain string

	// interval is the live poll interval (also drives the
	// /api/stats/online window); the settings PUT updates it.
	mu       sync.Mutex
	interval time.Duration
}

func (h *Handlers) setInterval(d time.Duration) {
	h.mu.Lock()
	h.interval = d
	h.mu.Unlock()
}

func (h *Handlers) currentInterval() time.Duration {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.interval
}

// --- settings ---

// SettingsGet serves GET /api/settings: whitelisted keys only.
func (h *Handlers) SettingsGet(w http.ResponseWriter, r *http.Request) {
	sec := int(h.currentInterval().Seconds())
	totp := false
	admin, err := h.Store.AdminGet()
	if err == nil {
		totp = admin.TOTPEnabled
	} else if !errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{pollIntervalKey: sec, "totp_enabled": totp})
}

// SettingsPut serves PUT /api/settings. Only poll_interval_sec is
// accepted (5–300); a valid value is stored and applied to the poller
// live. Unknown keys and out-of-range values answer 400.
func (h *Handlers) SettingsPut(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	for key := range body {
		if key != pollIntervalKey {
			jsonError(w, http.StatusBadRequest, "unknown_key")
			return
		}
	}
	raw := body[pollIntervalKey]
	if len(raw) == 0 {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	var v float64
	if err := json.Unmarshal(raw, &v); err != nil || v != float64(int(v)) {
		jsonError(w, http.StatusBadRequest, "invalid_value")
		return
	}
	sec := int(v)
	if sec < pollIntervalMin || sec > pollIntervalMax {
		jsonError(w, http.StatusBadRequest, "invalid_value")
		return
	}
	d := time.Duration(sec) * time.Second
	if err := h.Store.SettingSet(pollIntervalKey, strconv.Itoa(sec)); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	h.Poller.SetInterval(d)
	h.setInterval(d)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- xray lifecycle ---

// XrayStatus serves GET /api/xray/status.
func (h *Handlers) XrayStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Mgr.Status())
}

// XrayStart serves POST /api/xray/start.
func (h *Handlers) XrayStart(w http.ResponseWriter, r *http.Request) {
	h.xrayAction(w, "xray_start_failed", h.Mgr.Start)
}

// XrayStop serves POST /api/xray/stop.
func (h *Handlers) XrayStop(w http.ResponseWriter, r *http.Request) {
	h.xrayAction(w, "xray_stop_failed", h.Mgr.Stop)
}

// XrayRestart serves POST /api/xray/restart.
func (h *Handlers) XrayRestart(w http.ResponseWriter, r *http.Request) {
	h.xrayAction(w, "xray_restart_failed", h.Mgr.Restart)
}

func (h *Handlers) xrayAction(w http.ResponseWriter, code string, fn func() error) {
	if err := fn(); err != nil {
		jsonError(w, http.StatusInternalServerError, code)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// --- stats views ---

// StatsTraffic serves GET /api/stats/traffic?user=&range=day|week|month
// (range defaults to day; empty user aggregates all users).
func (h *Handlers) StatsTraffic(w http.ResponseWriter, r *http.Request) {
	rng := r.URL.Query().Get("range")
	if rng == "" {
		rng = "day"
	}
	buckets, err := stats.Traffic(h.Store, r.URL.Query().Get("user"), rng)
	if errors.Is(err, stats.ErrBadRange) {
		jsonError(w, http.StatusBadRequest, "bad_range")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, buckets)
}

// StatsOnline serves GET /api/stats/online: devices seen within the
// last two poll intervals.
func (h *Handlers) StatsOnline(w http.ResponseWriter, r *http.Request) {
	devices, err := h.Store.DevicesOnline(time.Now().UTC().Add(-2 * h.currentInterval()))
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

// StatsSummary serves GET /api/stats/summary.
func (h *Handlers) StatsSummary(w http.ResponseWriter, r *http.Request) {
	// The live poll interval drives the online window (2× interval),
	// matching /api/stats/online.
	up, down, online, err := stats.Summary(h.Store, h.currentInterval())
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"total_up": up, "total_down": down, "online_count": online,
	})
}

// --- user subscription URL (UI copy) ---

// UserSub serves GET /api/users/{id}/sub: the subscription URL and
// available formats for copy-paste in the UI.
func (h *Handlers) UserSub(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
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
	writeJSON(w, http.StatusOK, map[string]any{
		"url":     "https://" + h.PanelDomain + "/sub/" + u.SubToken,
		"formats": []string{"base64", "json", "mihomo", "clash"},
	})
}

// --- helpers shared with router.go ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// --- seed and poll interval resolution ---

// PollInterval resolves the effective poll interval: the stored
// poll_interval_sec setting when valid (5–300), else the config value,
// else the 30s default when the config value is non-positive.
func PollInterval(st *store.Store, cfg *config.Config) time.Duration {
	if v, err := st.SettingGet(pollIntervalKey); err == nil {
		if n, err := strconv.Atoi(v); err == nil && n >= pollIntervalMin && n <= pollIntervalMax {
			return time.Duration(n) * time.Second
		}
	}
	if cfg.PollIntervalSec > 0 {
		return time.Duration(cfg.PollIntervalSec) * time.Second
	}
	return defaultPollSec * time.Second
}

// Seed idempotently initializes first-run state: the default Reality
// inbound, the three subscription templates and the JWT signing key.
// Existing values are never overwritten. poll_interval_sec is not
// seeded — the env config stays the source until the admin sets it.
func Seed(st *store.Store) error {
	inbounds, err := st.InboundList()
	if err != nil {
		return err
	}
	if len(inbounds) == 0 {
		in, err := defaultInbound()
		if err != nil {
			return err
		}
		if err := st.InboundCreate(in); err != nil {
			return err
		}
	}
	for _, format := range []string{"json", "mihomo", "clash"} {
		if _, err := st.SettingGet("template_" + format); errors.Is(err, store.ErrNotFound) {
			if err := st.SettingSet("template_"+format, sub.DefaultTemplate(format)); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	if _, err := st.SettingGet(jwtKeySetting); errors.Is(err, store.ErrNotFound) {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return err
		}
		if err := st.SettingSet(jwtKeySetting, base64.StdEncoding.EncodeToString(key)); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

// defaultInbound builds the seeded VLESS + xhttp + Reality inbound
// (spec §6.4). settings.clients starts empty — the config generator
// fills it from the active users on every apply.
func defaultInbound() (*store.Inbound, error) {
	priv, pub, err := xray.NewKeyPair()
	if err != nil {
		return nil, err
	}
	sid := make([]byte, 4) // one random 8-hex shortId
	if _, err := rand.Read(sid); err != nil {
		return nil, err
	}
	cfg := map[string]any{
		"listen":   "0.0.0.0",
		"port":     8443,
		"tag":      "vless-xhttp",
		"protocol": "vless",
		"settings": map[string]any{"clients": []any{}, "decryption": "none"},
		"streamSettings": map[string]any{
			"network":  "xhttp",
			"security": "reality",
			"xhttpSettings": map[string]any{
				"path": "/xhttp", "mode": "stream-up",
			},
			"realitySettings": map[string]any{
				"dest":        "www.microsoft.com:443",
				"serverNames": []any{"www.microsoft.com"},
				"privateKey":  priv,
				"publicKey":   pub,
				"shortIds":    []any{hex.EncodeToString(sid)},
				"fingerprint": "chrome",
			},
		},
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return &store.Inbound{
		Tag: "vless-xhttp", Enabled: true, ConfigJSON: string(b),
		CreatedAt: time.Now().UTC(),
	}, nil
}
