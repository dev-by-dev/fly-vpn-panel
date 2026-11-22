package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flypanel/internal/auth"
	"flypanel/internal/store"
)

// maxTokenName bounds the display name; tokens themselves are generated.
const maxTokenName = 64

type tokenJSON struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Scope     string `json:"scope"`
	ExpiresAt string `json:"expires_at"`
	Revoked   bool   `json:"revoked"`
	CreatedAt string `json:"created_at"`
}

func tokenToJSON(t store.APIToken) tokenJSON {
	exp := ""
	if !t.ExpiresAt.IsZero() {
		exp = t.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return tokenJSON{
		ID: t.ID, Name: t.Name, Scope: t.Scope,
		ExpiresAt: exp, Revoked: t.Revoked,
		CreatedAt: t.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// ListTokens answers GET /api/tokens.
func (h *Handlers) ListTokens(w http.ResponseWriter, r *http.Request) {
	list, err := h.Store.APITokenList()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	out := make([]tokenJSON, 0, len(list))
	for _, t := range list {
		out = append(out, tokenToJSON(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": out})
}

type tokenCreateBody struct {
	Name          string `json:"name"`
	Scope         string `json:"scope"`
	ExpiresInDays int    `json:"expires_in_days"`
}

// CreateToken answers POST /api/tokens and returns the raw token once.
func (h *Handlers) CreateToken(w http.ResponseWriter, r *http.Request) {
	var body tokenCreateBody
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&body); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_token_request")
		return
	}
	name := strings.TrimSpace(body.Name)
	if name == "" || len([]rune(name)) > maxTokenName {
		jsonError(w, http.StatusBadRequest, "invalid_token_request")
		return
	}
	if body.Scope != "full" && body.Scope != "read" {
		jsonError(w, http.StatusBadRequest, "invalid_token_request")
		return
	}
	if body.ExpiresInDays != 0 && body.ExpiresInDays != 30 && body.ExpiresInDays != 90 {
		jsonError(w, http.StatusBadRequest, "invalid_token_request")
		return
	}
	var exp time.Time
	if body.ExpiresInDays > 0 {
		exp = time.Now().Add(time.Duration(body.ExpiresInDays) * 24 * time.Hour)
	}
	raw, hash, err := auth.IssueAPIToken()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	id, err := h.Store.APITokenInsert(name, body.Scope, hash, exp)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	expiresAt := ""
	if !exp.IsZero() {
		expiresAt = exp.UTC().Format(time.RFC3339)
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"id": id, "name": name, "scope": body.Scope,
		"expires_at": expiresAt, "token": raw,
	})
}

// RevokeToken answers DELETE /api/tokens/{id}.
func (h *Handlers) RevokeToken(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		jsonError(w, http.StatusBadRequest, "invalid_token_request")
		return
	}
	if err := h.Store.APITokenRevoke(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			jsonError(w, http.StatusNotFound, "not_found")
			return
		}
		jsonError(w, http.StatusInternalServerError, "internal")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
