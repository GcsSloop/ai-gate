package api

import "net/http"

// RequireProxyEnabled blocks gateway traffic when proxy mode is disabled.
func RequireProxyEnabled(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsProxyEnabled() {
			http.Error(w, "proxy is disabled; enable proxy in AI Gate to accept requests", http.StatusServiceUnavailable)
			return
		}
		next.ServeHTTP(w, r)
	})
}
