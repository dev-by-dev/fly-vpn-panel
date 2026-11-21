package sub

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"flypanel/internal/store"
)

// Handlers serves the public subscription endpoints
// (GET /sub/{token}[/{format}]) and the admin template API
// (GET/PUT /api/templates/{format}). Routing/auth wiring is Task 10.
type Handlers struct {
	Store   *store.Store
	EntryIP string
}

// maxTemplateSize caps a PUT body at 1 MiB.
const maxTemplateSize = 1 << 20

var formatCT = map[string]string{
	"base64": "text/plain",
	"json":   "application/json",
	"mihomo": "text/yaml",
	"clash":  "text/yaml",
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// recordDevice сохраняет заголовки клиента для списка устройств. Ошибки
// только логируем: подписка важнее статистики.
func (h *Handlers) recordDevice(r *http.Request, username string) {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		ip = host
	}
	// X-Forwarded-For доверяем только локальному Caddy.
	if ip == "127.0.0.1" || ip == "::1" {
		if first, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ","); strings.TrimSpace(first) != "" {
			ip = strings.TrimSpace(first)
		}
	}
	meta := store.DeviceMeta{
		Platform:  clipHeader(r.Header.Get("X-Device-OS")),
		OSVersion: clipHeader(r.Header.Get("X-Ver-OS")),
		Model:     clipHeader(r.Header.Get("X-Device-Model")),
		UserAgent: clipHeader(r.UserAgent()),
	}
	// Публичную ссылку дёргают боты и сканеры — устройством считаем
	// только запрос от VPN-клиента.
	if !looksLikeClient(meta) {
		return
	}
	if err := h.Store.DeviceUpsert(username, ip, meta, time.Now().UTC()); err != nil {
		log.Printf("sub: device record %q: %v", username, err)
	}
}

func clipHeader(s string) string {
	if len(s) > 256 {
		return s[:256]
	}
	return s
}

// Sub serves GET /sub/{token} and GET /sub/{token}/{format}. Unknown and
// deleted tokens return the identical 404 body (no existence oracle).
// An expired user is disabled in the store and gets 403 expired; a
// disabled user gets 403 disabled.
func (h *Handlers) Sub(w http.ResponseWriter, r *http.Request) {
	u, err := h.Store.UserGetByToken(r.PathValue("token"))
	if errors.Is(err, store.ErrNotFound) {
		jsonError(w, http.StatusNotFound, "not found")
		return
	}
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	now := time.Now().UTC()
	if u.Disabled {
		// A disabled user is never re-tagged here: overwriting the
		// reason (e.g. manual → expired) would let the poller
		// auto-enable a manually disabled user once the expiry is
		// extended. Only an "expired"-tagged user keeps the expired
		// response so clients still see why they were cut off.
		if u.DisabledReason == "expired" {
			jsonError(w, http.StatusForbidden, "expired")
			return
		}
		jsonError(w, http.StatusForbidden, "disabled")
		return
	}
	if !u.Expiry.After(now) {
		if err := h.Store.UserSetDisabled(u.ID, true, "expired"); err != nil {
			jsonError(w, http.StatusInternalServerError, "internal")
			return
		}
		jsonError(w, http.StatusForbidden, "expired")
		return
	}
	h.recordDevice(r, u.Username)

	format := r.PathValue("format")
	if format == "" {
		format = FormatForUA(r.UserAgent())
	} else if formatCT[format] == "" {
		jsonError(w, http.StatusBadRequest, "bad_format")
		return
	}
	body, err := h.render(format, u)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	up, down, err := h.Store.SamplesSince(u.Username, u.QuotaResetAt)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Subscription-Userinfo", fmt.Sprintf(
		"upload=%d; download=%d; total=%d; expire=%d", up, down, u.QuotaBytes, u.Expiry.Unix()))
	w.Header().Set("Content-Type", formatCT[format])
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

// render produces the subscription body for one format, using the
// stored template when set and the embedded default otherwise.
func (h *Handlers) render(format string, u *store.User) ([]byte, error) {
	inbounds, err := h.Store.InboundListEnabled()
	if err != nil {
		return nil, err
	}
	switch format {
	case "base64":
		return []byte(RenderBase64(inbounds, *u, h.EntryIP)), nil
	case "json":
		tpl, err := h.template("json")
		if err != nil {
			return nil, err
		}
		return RenderJSON(inbounds, *u, h.EntryIP, tpl)
	default: // mihomo, clash
		tpl, err := h.template(format)
		if err != nil {
			return nil, err
		}
		params, err := proxyParams(inbounds, *u, h.EntryIP)
		if err != nil {
			return nil, err
		}
		out, err := RenderYAML(tpl, params)
		if err != nil {
			return nil, err
		}
		return []byte(out), nil
	}
}

// template returns the stored template for format, falling back to the
// embedded default when the settings key is unset.
func (h *Handlers) template(format string) (string, error) {
	v, err := h.Store.SettingGet("template_" + format)
	if errors.Is(err, store.ErrNotFound) {
		return DefaultTemplate(format), nil
	}
	return v, err
}

// TemplateGet serves GET /api/templates/{format} (json|mihomo|clash),
// returning the stored or default template raw — never sanitized.
func (h *Handlers) TemplateGet(w http.ResponseWriter, r *http.Request) {
	format := r.PathValue("format")
	if DefaultTemplate(format) == "" {
		jsonError(w, http.StatusBadRequest, "bad_format")
		return
	}
	tpl, err := h.template(format)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = w.Write([]byte(tpl))
}

// TemplatePut serves PUT /api/templates/{format}: the body is stored
// raw. The only validation is a render test against a fictional
// TEST-NET user, so a template with broken placeholders is rejected
// (400) without any content sanitizing.
func (h *Handlers) TemplatePut(w http.ResponseWriter, r *http.Request) {
	format := r.PathValue("format")
	if DefaultTemplate(format) == "" {
		jsonError(w, http.StatusBadRequest, "bad_format")
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxTemplateSize+1))
	if err != nil {
		jsonError(w, http.StatusBadRequest, "bad_request")
		return
	}
	if len(body) > maxTemplateSize {
		jsonError(w, http.StatusRequestEntityTooLarge, "too_large")
		return
	}
	tpl := string(body)
	if err := renderTest(format, tpl); err != nil {
		jsonError(w, http.StatusBadRequest, "broken_template")
		return
	}
	if err := h.Store.SettingSet("template_"+format, tpl); err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Render-test fixtures: TEST-NET address, example.com SNI, fictional
// user; www.microsoft.com:443 is the spec's Reality dest constant.
var (
	tplTestUser    = store.User{Username: "tpltestuser", UUID: "123e4567-e89b-42d3-a456-426614174000"}
	tplTestEntryIP = "203.0.113.10"
	tplTestInbound = store.Inbound{
		Tag: "tpl-test-in", Enabled: true,
		ConfigJSON: `{"port":443,"tag":"tpl-test-in","streamSettings":{"network":"xhttp","security":"reality","realitySettings":{"dest":"www.microsoft.com:443","serverNames":["example.com"],"privateKey":"tpltestpriv","publicKey":"tpltestpbk","shortIds":["tpltestsid"],"fingerprint":"chrome"},"xhttpSettings":{"path":"/tpltestpath","mode":"auto"}}}`,
	}
)

// renderTest verifies a template still renders: every placeholder that
// exists in it must substitute cleanly on the fictional user.
func renderTest(format, tpl string) error {
	if format == "json" {
		_, err := RenderJSON([]store.Inbound{tplTestInbound}, tplTestUser, tplTestEntryIP, tpl)
		return err
	}
	params, err := proxyParams([]store.Inbound{tplTestInbound}, tplTestUser, tplTestEntryIP)
	if err != nil {
		return err
	}
	_, err = RenderYAML(tpl, params)
	return err
}

// looksLikeClient отсекает ботов и сканеры: подписка открыта всем, но
// устройством является только запрос от VPN-клиента.
func looksLikeClient(meta store.DeviceMeta) bool {
	if meta.Platform != "" || meta.OSVersion != "" || meta.Model != "" {
		return true
	}
	ua := strings.ToLower(meta.UserAgent)
	for _, name := range []string{
		"happ", "incy", "v2rayng", "v2ray", "clash", "mihomo", "hiddify",
		"sing-box", "nekobox", "streisand", "shadowrocket", "karing", "sfa",
	} {
		if strings.Contains(ua, name) {
			return true
		}
	}
	return false
}
