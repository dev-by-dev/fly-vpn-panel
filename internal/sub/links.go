// Package sub renders user subscriptions in four formats (base64 link
// list, xray JSON, mihomo yaml, clash yaml) and serves /sub and
// /api/templates.
package sub

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"flypanel/internal/store"
)

// linkParams are the per-inbound values baked into every client format.
type linkParams struct {
	Tag  string
	Port int
	PBK  string
	SID  string
	SNI  string
	FP   string
	Path string
	Mode string
}

// inboundParams extracts the link fields from a raw xray inbound config
// (Inbound.ConfigJSON). fp defaults to "chrome" when absent; pbk/sid/sni
// take the first of their arrays.
func inboundParams(in store.Inbound) (linkParams, error) {
	var m map[string]any
	if err := json.Unmarshal([]byte(in.ConfigJSON), &m); err != nil {
		return linkParams{}, fmt.Errorf("inbound %s: bad config: %w", in.Tag, err)
	}
	p := linkParams{Tag: in.Tag}
	port, ok := m["port"].(float64)
	if !ok || port < 1 || port > 65535 {
		return linkParams{}, fmt.Errorf("inbound %s: missing or invalid port", in.Tag)
	}
	p.Port = int(port)
	ss, _ := m["streamSettings"].(map[string]any)
	if ss == nil {
		return linkParams{}, fmt.Errorf("inbound %s: missing streamSettings", in.Tag)
	}
	rs, _ := ss["realitySettings"].(map[string]any)
	xs, _ := ss["xhttpSettings"].(map[string]any)
	if rs == nil || xs == nil {
		return linkParams{}, fmt.Errorf("inbound %s: missing reality/xhttp settings", in.Tag)
	}
	p.PBK = str(rs["publicKey"])
	p.SID = firstString(rs["shortIds"])
	p.SNI = firstString(rs["serverNames"])
	p.FP = str(rs["fingerprint"])
	p.Path = str(xs["path"])
	p.Mode = str(xs["mode"])
	if p.PBK == "" || p.SID == "" || p.SNI == "" || p.Path == "" || p.Mode == "" {
		return linkParams{}, fmt.Errorf("inbound %s: incomplete reality/xhttp settings", in.Tag)
	}
	if p.FP == "" {
		p.FP = "chrome"
	}
	return p, nil
}

// RenderBase64 encodes one vless link per enabled inbound as
// base64(list joined by "\n"). Inbounds that fail to parse are skipped
// (the string signature has no error path); RenderJSON surfaces the error.
func RenderBase64(inbounds []store.Inbound, u store.User, entryIP string) string {
	lines := make([]string, 0, len(inbounds))
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		p, err := inboundParams(in)
		if err != nil {
			continue
		}
		q := url.Values{}
		q.Set("type", "xhttp")
		q.Set("path", p.Path)
		q.Set("mode", p.Mode)
		q.Set("security", "reality")
		q.Set("pbk", p.PBK)
		q.Set("sid", p.SID)
		q.Set("sni", p.SNI)
		q.Set("fp", p.FP)
		q.Set("encryption", "none")
		lines = append(lines, fmt.Sprintf("vless://%s@%s:%d?%s#%s",
			u.UUID, entryIP, p.Port, q.Encode(), url.PathEscape(p.Tag)))
	}
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(lines, "\n")))
}

func str(v any) string {
	s, _ := v.(string)
	return s
}

func firstString(v any) string {
	arr, _ := v.([]any)
	if len(arr) == 0 {
		return ""
	}
	return str(arr[0])
}
