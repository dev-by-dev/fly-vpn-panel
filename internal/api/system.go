package api

import (
	"net/http"

	"flypanel/internal/host"
)

// System serves GET /api/system: host CPU, memory and disk usage for
// the main page. Metrics the host cannot provide are zero.
func (h *Handlers) System(w http.ResponseWriter, r *http.Request) {
	s, err := host.Read(h.DataDir)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	writeJSON(w, http.StatusOK, s)
}
