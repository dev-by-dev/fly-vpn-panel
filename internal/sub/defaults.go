package sub

import _ "embed"

// Mirrored from the production panel's client templates (spec §6.6), with
// the generator's injection markers replaced by our raw {{KEY}} placeholders.

//go:embed defaults/default_json.json
var defaultJSON string

//go:embed defaults/default_mihomo.yaml
var defaultMihomo string

//go:embed defaults/default_clash.yaml
var defaultClash string

// DefaultTemplate returns the embedded default template for format
// ("json", "mihomo", "clash"), or "" for formats without a template
// (base64 is generated structurally, no template exists).
func DefaultTemplate(format string) string {
	switch format {
	case "json":
		return defaultJSON
	case "mihomo":
		return defaultMihomo
	case "clash":
		return defaultClash
	}
	return ""
}
