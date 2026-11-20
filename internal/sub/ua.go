package sub

import "strings"

// FormatForUA maps a client User-Agent to a subscription format:
// v2rayNG → base64, FlClash → mihomo, clash-verge (raw, CamelCase must
// not match) → mihomo, everything else → base64.
func FormatForUA(ua string) string {
	lower := strings.ToLower(ua)
	switch {
	case strings.Contains(lower, "v2rayng"):
		return "base64"
	case strings.Contains(lower, "flclash"):
		return "mihomo"
	case strings.Contains(ua, "clash-verge"):
		return "mihomo"
	default:
		return "base64"
	}
}
