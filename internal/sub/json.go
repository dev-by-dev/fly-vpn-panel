package sub

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"flypanel/internal/store"
)

// RenderJSON substitutes the per-inbound values into templateJSON (an
// xray client config carrying {{UUID}} {{ADDRESS}} {{PORT}} {{PBK}}
// {{SID}} {{SNI}} {{FP}} {{PATH}} {{MODE}} {{TAG}}) once per enabled
// inbound, returning a JSON array of complete client configs. A leftover
// or unknown placeholder, or a rendered config that is not valid JSON,
// is an error. Substitution is raw text: template content is never
// parsed or rewritten (anti-sanitizer guarantee).
func RenderJSON(inbounds []store.Inbound, u store.User, entryIP, templateJSON string) ([]byte, error) {
	parts := make([]string, 0, len(inbounds))
	for _, in := range inbounds {
		if !in.Enabled {
			continue
		}
		p, err := inboundParams(in)
		if err != nil {
			return nil, err
		}
		out := templateJSON
		// Single-pass replacer: substituted values are never re-scanned,
		// so a value containing {{KEY}} stays verbatim (and trips the
		// leftover-placeholder check below) instead of being
		// re-substituted with that key's value.
		repl := strings.NewReplacer(
			"{{UUID}}", u.UUID,
			"{{ADDRESS}}", entryIP,
			"{{PORT}}", strconv.Itoa(p.Port),
			"{{PBK}}", p.PBK,
			"{{SID}}", p.SID,
			"{{SNI}}", p.SNI,
			"{{FP}}", p.FP,
			"{{PATH}}", p.Path,
			"{{MODE}}", p.Mode,
			"{{TAG}}", p.Tag,
		)
		out = repl.Replace(out)
		if strings.Contains(out, "{{") {
			return nil, fmt.Errorf("rendered config %q: unresolved placeholder", p.Tag)
		}
		if !json.Valid([]byte(out)) {
			return nil, fmt.Errorf("rendered config %q: invalid JSON", p.Tag)
		}
		parts = append(parts, out)
	}
	return []byte("[" + strings.Join(parts, ",") + "]"), nil
}
