package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/policy"
)

type PolicyHandler struct {
	repo policy.Repository
}

func NewPolicyHandler(repo policy.Repository) *PolicyHandler {
	return &PolicyHandler{repo: repo}
}

func (h *PolicyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/policy/")
	if name == "" || name == r.URL.Path {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		def, err := h.repo.Get(name)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, def)
	case http.MethodPut:
		var def policy.Definition
		if err := json.NewDecoder(r.Body).Decode(&def); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if def.Name == "" {
			def.Name = name
		}
		if def.TokenBudgetFactor <= 0 {
			http.Error(w, "token_budget_factor must be positive", http.StatusBadRequest)
			return
		}
		if err := h.repo.Save(def); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, def)
	default:
		http.NotFound(w, r)
	}
}
