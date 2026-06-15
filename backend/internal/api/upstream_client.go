package api

import (
	"net/http"

	"github.com/gcssloop/codex-router/backend/internal/accounts"
	"github.com/gcssloop/codex-router/backend/internal/netproxy"
)

func doAccountRequest(client *http.Client, req *http.Request, account accounts.Account) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if account.SkipTLSVerify {
		req = req.WithContext(netproxy.ContextWithSkipTLSVerify(req.Context(), true))
	}
	return client.Do(req)
}
