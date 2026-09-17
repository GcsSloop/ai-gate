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
	ctx := req.Context()
	if account.SkipTLSVerify {
		ctx = netproxy.ContextWithSkipTLSVerify(ctx, true)
	}
	if override := netproxy.ProxyOverrideForAccountMode(string(account.ProxyMode)); override != "" {
		ctx = netproxy.ContextWithProxyOverride(ctx, override)
	}
	if ctx != req.Context() {
		req = req.WithContext(ctx)
	}
	return client.Do(req)
}
