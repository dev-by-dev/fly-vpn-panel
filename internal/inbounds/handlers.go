package inbounds

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"flypanel/internal/store"
	"flypanel/internal/xray"
)

// maxBodyBytes bounds request bodies (inbound configs and routing JSON).
const maxBodyBytes = 64 << 10

// Handlers serves the /api/inbounds and /api/config endpoints.
type Handlers struct {
	Store   *store.Store
	Applier *Applier
}

type inboundResponse struct {
	ID        int64           `json:"id"`
	Tag       string          `json:"tag"`
	Enabled   bool            `json:"enabled"`
	Config    json.RawMessage `json:"config"`
	CreatedAt time.Time       `json:"created_at"`
}

type inboundRequest struct {
	Tag     string          `json:"tag"`
	Enabled *bool           `json:"enabled"`
	Config  json.RawMessage `json:"config"`
}

type inboundPatch struct {
	Tag     *string         `json:"tag"`
	Enabled *bool           `json:"enabled"`
	Config  json.RawMessage `json:"config"`
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
	return json.NewDecoder(io.LimitReader(r.Body, maxBodyBytes)).Decode(v)
}

func inboundID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func respondInbound(w http.ResponseWriter, status int, in *store.Inbound) {
	writeJSON(w, status, inboundResponse{
		ID: in.ID, Tag: in.Tag, Enabled: in.Enabled,
		Config: json.RawMessage(in.ConfigJSON), CreatedAt: in.CreatedAt,
	})
}

// applyCurrent regenerates and applies the config using the stored
// routing (xray's default when never set).
func (h *Handlers) applyCurrent() error {
	val, err := h.Store.SettingGet(routingKey)
	if errors.Is(err, store.ErrNotFound) {
		val = ""
	} else if err != nil {
		return err
	}
	return h.Applier.Apply(val)
}

// routing returns the stored routing value (default when unset) and
// whether a value had been stored.
func (h *Handlers) routing() (string, bool, error) {
	val, err := h.Store.SettingGet(routingKey)
	if errors.Is(err, store.ErrNotFound) {
		return defaultRouting, false, nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// realitySettings extracts streamSettings.realitySettings, or nil when
// the inbound does not use Reality.
func realitySettings(cfg map[string]any) map[string]any {
	ss, _ := cfg["streamSettings"].(map[string]any)
	rs, _ := ss["realitySettings"].(map[string]any)
	return rs
}

// ensureRealityKeys generates a Reality X25519 pair for a config whose
// realitySettings has empty privateKey AND publicKey: xray refuses to
// run a Reality inbound without keys, so a bare create would 502 on
// apply. Any other shape is returned unchanged.
func ensureRealityKeys(cfgJSON string) string {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		return cfgJSON
	}
	rs := realitySettings(cfg)
	if rs == nil {
		return cfgJSON
	}
	priv, _ := rs["privateKey"].(string)
	pub, _ := rs["publicKey"].(string)
	if priv != "" || pub != "" {
		return cfgJSON
	}
	newPriv, newPub, err := xray.NewKeyPair()
	if err != nil {
		return cfgJSON
	}
	rs["privateKey"] = newPriv
	rs["publicKey"] = newPub
	out, err := json.Marshal(cfg)
	if err != nil {
		return cfgJSON
	}
	return string(out)
}

// normalizeConfig validates and canonicalizes an inbound config: it must
// be a JSON object with port 1–65535, and a Reality inbound must have a
// non-empty dest. The panel tag is forced into the config because xray
// identifies inbounds by that tag and membership sync depends on the
// store tag and the xray tag matching.
func normalizeConfig(raw json.RawMessage, tag string) (string, string) {
	var cfg map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil || cfg == nil {
		return "", "invalid_config"
	}
	port, ok := cfg["port"].(float64)
	if !ok || port != math.Trunc(port) || port < 1 || port > 65535 {
		return "", "invalid_port"
	}
	cfg["tag"] = tag
	if rs := realitySettings(cfg); rs != nil {
		if dest, _ := rs["dest"].(string); dest == "" {
			return "", "reality_dest_required"
		}
	}
	out, err := json.Marshal(cfg)
	if err != nil {
		return "", "invalid_config"
	}
	return string(out), ""
}

// List serves GET /api/inbounds.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.InboundList()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	out := make([]inboundResponse, 0, len(list))
	for i := range list {
		in := list[i]
		out = append(out, inboundResponse{
			ID: in.ID, Tag: in.Tag, Enabled: in.Enabled,
			Config: json.RawMessage(in.ConfigJSON), CreatedAt: in.CreatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// Get serves GET /api/inbounds/{id}.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id, err := inboundID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	in, err := h.Store.InboundGet(id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	respondInbound(w, http.StatusOK, in)
}

// Create serves POST /api/inbounds. Form contract: unique tag, port
// 1–65535, non-empty Reality dest.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	var req inboundRequest
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if req.Tag == "" {
		jsonError(w, http.StatusBadRequest, "tag_required")
		return
	}
	if len(req.Config) == 0 {
		jsonError(w, http.StatusBadRequest, "config_required")
		return
	}
	cfgJSON, code := normalizeConfig(req.Config, req.Tag)
	if code != "" {
		jsonError(w, http.StatusBadRequest, code)
		return
	}
	// A Reality inbound without keys is rejected by `xray run -test`;
	// generate the pair up front so every create can apply.
	cfgJSON = ensureRealityKeys(cfgJSON)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	in := &store.Inbound{
		Tag: req.Tag, Enabled: enabled, ConfigJSON: cfgJSON,
		CreatedAt: time.Now().UTC(),
	}
	if err := h.Store.InboundCreate(in); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonError(w, http.StatusBadRequest, "duplicate_tag")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	// The row is committed before the apply; on apply failure the DB stays
	// the source of truth (no rollback) and the admin or poller retries —
	// the running xray just keeps serving the previous config.
	if err := h.applyCurrent(); err != nil {
		jsonError(w, http.StatusBadGateway, "apply_failed")
		return
	}
	respondInbound(w, http.StatusCreated, in)
}

// Patch serves PATCH /api/inbounds/{id} for tag, enabled and the full
// inbound config.
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id, err := inboundID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	var req inboundPatch
	if err := decodeJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	in, err := h.Store.InboundGet(id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if req.Tag != nil {
		if *req.Tag == "" {
			jsonError(w, http.StatusBadRequest, "tag_required")
			return
		}
		in.Tag = *req.Tag
	}
	if req.Enabled != nil {
		in.Enabled = *req.Enabled
	}
	if len(req.Config) > 0 {
		cfgJSON, code := normalizeConfig(req.Config, in.Tag)
		if code != "" {
			jsonError(w, http.StatusBadRequest, code)
			return
		}
		in.ConfigJSON = cfgJSON
	} else if req.Tag != nil {
		// Tag-only patch: re-normalize the stored config under the new
		// tag (normalizeConfig forces config tag) so store tag ==
		// xray tag — membership sync depends on them matching.
		cfgJSON, code := normalizeConfig(json.RawMessage(in.ConfigJSON), in.Tag)
		if code != "" {
			jsonError(w, http.StatusBadRequest, code)
			return
		}
		in.ConfigJSON = cfgJSON
	}
	if err := h.Store.InboundUpdate(in); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found")
			return
		}
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonError(w, http.StatusBadRequest, "duplicate_tag")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	// Same failure semantics as Create: the row is already updated, apply
	// failure is surfaced as 502 without a rollback.
	if err := h.applyCurrent(); err != nil {
		jsonError(w, http.StatusBadGateway, "apply_failed")
		return
	}
	respondInbound(w, http.StatusOK, in)
}

// Delete serves DELETE /api/inbounds/{id}.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := inboundID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	if err := h.Store.InboundDelete(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	// The row is gone; on apply failure xray keeps the old inbound until
	// the next successful apply (admin/poller retry, no resurrection).
	if err := h.applyCurrent(); err != nil {
		jsonError(w, http.StatusBadGateway, "apply_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// RegenKeys serves POST /api/inbounds/{id}/keys: a fresh Reality X25519
// pair replaces privateKey/publicKey in the inbound config, then the
// config is re-applied so xray picks up the new keys.
func (h *Handlers) RegenKeys(w http.ResponseWriter, r *http.Request) {
	id, err := inboundID(r)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_id")
		return
	}
	in, err := h.Store.InboundGet(id)
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(in.ConfigJSON), &cfg); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	rs := realitySettings(cfg)
	if rs == nil {
		jsonError(w, http.StatusBadRequest, "not_reality")
		return
	}
	priv, pub, err := xray.NewKeyPair()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	rs["privateKey"] = priv
	rs["publicKey"] = pub
	out, err := json.Marshal(cfg)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	in.ConfigJSON = string(out)
	if err := h.Store.InboundUpdate(in); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	// New keys only reach xray on a successful apply, but the DB keeps
	// them: no rollback, the next apply (retry) activates them.
	if err := h.applyCurrent(); err != nil {
		jsonError(w, http.StatusBadGateway, "apply_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"private_key": priv, "public_key": pub})
}

// GetConfig serves GET /api/config: the current xray.json as-is.
func (h *Handlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(h.Applier.ConfigPath())
	if errors.Is(err, fs.ErrNotExist) {
		jsonError(w, http.StatusNotFound, "not_found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}

// GetRouting serves GET /api/config/routing, returning the stored value
// verbatim (the default before the first PUT).
func (h *Handlers) GetRouting(w http.ResponseWriter, r *http.Request) {
	val, _, err := h.routing()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(val))
}

// PutRouting serves PUT /api/config/routing: a syntax-only JSON check,
// then the value is stored verbatim and applied. No sanitizer runs —
// full routing control is the product's core promise (spec §6.5).
func (h *Handlers) PutRouting(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if !json.Valid(body) {
		jsonError(w, http.StatusUnprocessableEntity, "invalid_json")
		return
	}
	// "null" (or any non-object) is syntactically valid JSON but not a
	// routing object: GenerateConfig would fail after the setting is
	// committed, and a panic there skips the rollback entirely — so
	// reject it with 422 BEFORE SettingSet.
	var probe map[string]any
	if err := json.Unmarshal(body, &probe); err != nil || probe == nil {
		jsonError(w, http.StatusUnprocessableEntity, "invalid_routing")
		return
	}
	old, hadOld, err := h.routing()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.Store.SettingSet(routingKey, string(body)); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	if err := h.Applier.Apply(string(body)); err != nil {
		// The new value never reached xray; restore the previous value so
		// the setting keeps describing the running config. A never-set
		// routing is restored to the default, which GET serves identically.
		rollback := defaultRouting
		if hadOld {
			rollback = old
		}
		if err := h.Store.SettingSet(routingKey, rollback); err != nil {
			jsonError(w, http.StatusInternalServerError, "rollback_failed")
			return
		}
		jsonError(w, http.StatusUnprocessableEntity, "apply_failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
