package sub

import (
	"fmt"
	"strings"

	"flypanel/internal/store"
)

// RenderYAML substitutes {{KEY}} placeholders as raw text — the yaml is
// never parsed, so template content is opaque to the panel. Any leftover
// {{ after substitution is an error.
func RenderYAML(template string, params map[string]string) (string, error) {
	out := template
	for k, v := range params {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	if strings.Contains(out, "{{") {
		return "", fmt.Errorf("unresolved placeholder in template")
	}
	return out, nil
}

// proxyParams renders the mihomo/clash {{PROXIES}} and {{PROXY_NAMES}}
// values for the enabled inbounds: one vless+xhttp+reality proxy entry
// per inbound, named by inbound tag, plus the quoted group membership
// list. With no inbounds the placeholders render as empty lists.
func proxyParams(inbounds []store.Inbound, u store.User, entryIP string) (map[string]string, error) {
	entries := make([]string, 0, len(inbounds))
	names := make([]string, 0, len(inbounds))
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		p, err := inboundParams(in)
		if err != nil {
			return nil, err
		}
		line := fmt.Sprintf(
			"  - {name: %s, type: vless, server: %s, port: %d, uuid: %s, network: %s, tls: true, udp: true, servername: %s, client-fingerprint: %s, reality-opts: {public-key: %s, short-id: %s}",
			yamlQuote(p.Tag), yamlQuote(entryIP), p.Port, yamlQuote(u.UUID), p.Net,
			yamlQuote(p.SNI), yamlQuote(p.FP), yamlQuote(p.PBK), yamlQuote(p.SID))
		if p.Net == "xhttp" {
			line += fmt.Sprintf(", xhttp-opts: {path: %s, mode: %s}", yamlQuote(p.Path), yamlQuote(p.Mode))
		}
		entries = append(entries, line+"}")
		names = append(names, yamlQuote(p.Tag))
	}
	proxies := "  []"
	if len(entries) > 0 {
		proxies = strings.Join(entries, "\n")
	}
	group := "[]"
	if len(names) > 0 {
		group = "[" + strings.Join(names, ", ") + "]"
	}
	return map[string]string{"PROXIES": proxies, "PROXY_NAMES": group}, nil
}

// yamlQuote double-quotes s for a flow-style yaml scalar.
func yamlQuote(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}
