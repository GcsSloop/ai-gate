package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/api"
	"github.com/gcssloop/codex-router/backend/internal/policy"
)

func TestPolicyHandler(t *testing.T) {
	t.Parallel()

	repo := policy.NewMemoryRepository()
	handler := api.NewPolicyHandler(repo)

	saveReq := httptest.NewRequest(http.MethodPut, "/policy/default", bytes.NewBufferString(`{
		"name":"default",
		"candidate_order":["account-a","account-b"],
		"minimum_balance_threshold":5,
		"minimum_quota_threshold":1000,
		"token_budget_factor":1.3,
		"model_pool_rules":{"gpt-4.1":["pool-primary"]}
	}`))
	saveReq.Header.Set("Content-Type", "application/json")
	saveRec := httptest.NewRecorder()
	handler.ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("PUT /policy/default status = %d, want %d", saveRec.Code, http.StatusOK)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/policy/default", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /policy/default status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var got policy.Definition
	if err := json.Unmarshal(getRec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	if got.TokenBudgetFactor != 1.3 {
		t.Fatalf("TokenBudgetFactor = %v, want %v", got.TokenBudgetFactor, 1.3)
	}

	invalidReq := httptest.NewRequest(http.MethodPut, "/policy/default", bytes.NewBufferString(`{"name":"default","token_budget_factor":0}`))
	invalidReq.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("invalid PUT status = %d, want %d", invalidRec.Code, http.StatusBadRequest)
	}
}
