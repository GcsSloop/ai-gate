package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerRewritesEmbeddedIndexAssetPrefix(t *testing.T) {
	handler := Handler("/ai-router")
	req := httptest.NewRequest(http.MethodGet, "/ai-router/webui/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /ai-router/webui/ status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/ai-router/webui/assets/") {
		t.Fatalf("index body missing ai-router asset prefix: %s", body)
	}
	if strings.Contains(body, "/ai-gate/webui/assets/") {
		t.Fatalf("index body still contains ai-gate asset prefix: %s", body)
	}
}
