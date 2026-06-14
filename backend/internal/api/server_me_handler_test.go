package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/serverauth"
	"github.com/gcssloop/codex-router/backend/internal/serverusers"
	sqlitestore "github.com/gcssloop/codex-router/backend/internal/store/sqlite"
	"github.com/gcssloop/codex-router/backend/internal/usage"
)

func TestServerMeHandlerReturnsOwnUsageAndAssignedAccounts(t *testing.T) {
	store, err := sqlitestore.Open(filepath.Join(t.TempDir(), "router.sqlite"))
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	accountRepo := accounts.NewSQLiteRepository(store.DB())
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "assigned",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-assigned",
		BaseURL:       "https://assigned.example.invalid/v1",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create assigned account returned error: %v", err)
	}
	if err := accountRepo.Create(accounts.Account{
		ProviderType:  accounts.ProviderOpenAICompatible,
		AccountName:   "unassigned",
		AuthMode:      accounts.AuthModeAPIKey,
		CredentialRef: "sk-unassigned",
		BaseURL:       "https://unassigned.example.invalid/v1",
		Status:        accounts.StatusActive,
	}); err != nil {
		t.Fatalf("Create unassigned account returned error: %v", err)
	}
	accountsList, err := accountRepo.List()
	if err != nil {
		t.Fatalf("List accounts returned error: %v", err)
	}
	assignedID := accountsList[0].ID
	unassignedID := accountsList[1].ID

	userRepo := serverusers.NewSQLiteRepository(store.DB())
	created, err := userRepo.Create("alice")
	if err != nil {
		t.Fatalf("Create user returned error: %v", err)
	}
	if err := userRepo.SetAccountAssignments(created.User.ID, []int64{assignedID}); err != nil {
		t.Fatalf("SetAccountAssignments returned error: %v", err)
	}
	userID := created.User.ID
	usageRepo := usage.NewSQLiteRepository(store.DB())
	if err := usageRepo.SaveEvent(usage.Event{
		AccountID:     assignedID,
		ServerUserID:  &userID,
		ProviderType:  "openai-compatible",
		RequestKind:   "responses",
		Model:         "gpt-test",
		Status:        "completed",
		InputTokens:   10,
		OutputTokens:  20,
		TotalTokens:   30,
		EstimatedCost: 0.01,
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveEvent returned error: %v", err)
	}

	handler := NewServerMeHandler(userRepo)
	session := serverauth.Session{Authenticated: true, Role: serverusers.RoleUser, UserID: created.User.ID, Username: "alice"}

	meReq := httptest.NewRequest(http.MethodGet, "/me", nil)
	meReq = meReq.WithContext(serverauth.ContextWithSession(meReq.Context(), session))
	meRec := httptest.NewRecorder()
	handler.ServeHTTP(meRec, meReq)
	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /me status = %d, want %d; body=%s", meRec.Code, http.StatusOK, meRec.Body.String())
	}
	var me struct {
		User         serverusers.User `json:"user"`
		RequestCount int64            `json:"request_count"`
		TotalTokens  int64            `json:"total_tokens"`
	}
	if err := json.Unmarshal(meRec.Body.Bytes(), &me); err != nil {
		t.Fatalf("unmarshal me: %v", err)
	}
	if me.User.ID != created.User.ID || me.RequestCount != 1 || me.TotalTokens != 30 {
		t.Fatalf("me = %+v, want own usage", me)
	}

	accountsReq := httptest.NewRequest(http.MethodGet, "/me/accounts", nil)
	accountsReq = accountsReq.WithContext(serverauth.ContextWithSession(accountsReq.Context(), session))
	accountsRec := httptest.NewRecorder()
	handler.ServeHTTP(accountsRec, accountsReq)
	if accountsRec.Code != http.StatusOK {
		t.Fatalf("GET /me/accounts status = %d, want %d; body=%s", accountsRec.Code, http.StatusOK, accountsRec.Body.String())
	}
	var assigned []serverusers.AssignedAccount
	if err := json.Unmarshal(accountsRec.Body.Bytes(), &assigned); err != nil {
		t.Fatalf("unmarshal assigned accounts: %v", err)
	}
	if len(assigned) != 1 || assigned[0].AccountID != assignedID || assigned[0].CredentialRef != "" {
		t.Fatalf("assigned = %+v, want only assigned account without credential", assigned)
	}

	stateReq := httptest.NewRequest(http.MethodPut, "/me/accounts/"+strconv.FormatInt(assignedID, 10)+"/state", bytes.NewBufferString(`{"position":3,"is_active":true,"is_locked":true}`))
	stateReq.Header.Set("Content-Type", "application/json")
	stateReq = stateReq.WithContext(serverauth.ContextWithSession(stateReq.Context(), session))
	stateRec := httptest.NewRecorder()
	handler.ServeHTTP(stateRec, stateReq)
	if stateRec.Code != http.StatusOK {
		t.Fatalf("PUT assigned state status = %d, want %d; body=%s", stateRec.Code, http.StatusOK, stateRec.Body.String())
	}

	unassignedReq := httptest.NewRequest(http.MethodPut, "/me/accounts/"+strconv.FormatInt(unassignedID, 10)+"/state", bytes.NewBufferString(`{"position":0,"is_active":true,"is_locked":false}`))
	unassignedReq.Header.Set("Content-Type", "application/json")
	unassignedReq = unassignedReq.WithContext(serverauth.ContextWithSession(unassignedReq.Context(), session))
	unassignedRec := httptest.NewRecorder()
	handler.ServeHTTP(unassignedRec, unassignedReq)
	if unassignedRec.Code != http.StatusNotFound {
		t.Fatalf("PUT unassigned state status = %d, want %d", unassignedRec.Code, http.StatusNotFound)
	}
}
