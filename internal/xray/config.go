// Package xray generates xray-core configs, validates them, manages the
// xray child process lifecycle, and creates Reality key pairs.
package xray

import (
	"encoding/json"
	"fmt"

	"flypanel/internal/store"
)

// defaultRouting is used when routingJSON is empty.
const defaultRouting = `{"domainStrategy":"AsIs","rules":[]}`

// GenerateConfig renders the full xray-core config: the API service and
// stats policy, the dokodemo-door API inbound on 127.0.0.1:apiPort, every
// enabled inbound with the non-disabled users injected as VLESS clients,
// and freedom/blackhole outbounds. An empty egressIP omits sendThrough.
func GenerateConfig(inbounds []store.Inbound, activeUsers []store.User, routingJSON, egressIP string, apiPort int) ([]byte, error) {
	routing, err := parseRouting(routingJSON)
	if err != nil {
		return nil, err
	}

	clients := make([]any, 0, len(activeUsers))
	for _, u := range activeUsers {
		if u.Disabled {
			continue
		}
		clients = append(clients, map[string]any{"id": u.UUID, "email": u.Username, "level": 0})
	}

	inList := []any{
		map[string]any{
			"tag":      "api",
			"listen":   "127.0.0.1",
			"port":     apiPort,
			"protocol": "dokodemo-door",
			"settings": map[string]any{"address": "127.0.0.1"},
		},
	}
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		var ib map[string]any
		if err := json.Unmarshal([]byte(in.ConfigJSON), &ib); err != nil {
			return nil, fmt.Errorf("inbound %q config: %w", in.Tag, err)
		}
		settings, _ := ib["settings"].(map[string]any)
		if settings == nil {
			settings = map[string]any{}
			ib["settings"] = settings
		}
		settings["clients"] = clients
		// xray 26.x rejects vless inbounds without decryption:"none".
		if ib["protocol"] == "vless" {
			if _, ok := settings["decryption"]; !ok {
				settings["decryption"] = "none"
			}
		}
		inList = append(inList, ib)
	}

	freedom := map[string]any{"tag": "direct", "protocol": "freedom"}
	if egressIP != "" {
		freedom["sendThrough"] = egressIP
	}

	cfg := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"api": map[string]any{
			"tag":      "api",
			"services": []any{"HandlerService", "StatsService"},
		},
		"policy": map[string]any{
			"levels": map[string]any{
				"0": map[string]any{
					"statsUserUplink":   true,
					"statsUserDownlink": true,
					"statsUserOnline":   true,
				},
			},
			"system": map[string]any{
				"statsInboundUplink":   true,
				"statsInboundDownlink": true,
			},
		},
		"inbounds": inList,
		"outbounds": []any{
			freedom,
			map[string]any{"tag": "block", "protocol": "blackhole"},
		},
		"routing": routing,
	}

	out, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal config: %w", err)
	}
	return append(out, '\n'), nil
}

// parseRouting decodes routingJSON, substituting the default for an empty
// string and failing on invalid JSON.
func parseRouting(routingJSON string) (map[string]any, error) {
	if routingJSON == "" {
		routingJSON = defaultRouting
	}
	var routing map[string]any
	if err := json.Unmarshal([]byte(routingJSON), &routing); err != nil {
		return nil, fmt.Errorf("routing: %w", err)
	}
	return routing, nil
}
