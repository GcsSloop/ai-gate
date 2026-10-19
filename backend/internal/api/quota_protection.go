package api

import (
	"errors"
	"net/http"
)

var errQuotaProtected = errors.New("official account quota protection: the final 3% of the 5-hour or overall quota is reserved; no alternative account is available")

func writeQuotaProtected(w http.ResponseWriter) {
	writeJSON(w, http.StatusTooManyRequests, map[string]any{
		"error": map[string]any{
			"type":    "rate_limit_error",
			"code":    "quota_protected",
			"message": errQuotaProtected.Error(),
		},
	})
}
