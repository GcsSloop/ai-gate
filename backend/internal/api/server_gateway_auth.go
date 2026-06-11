package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/gcssloop/codex-router/backend/internal/serverusers"
)

type ServerGatewayUserStore interface {
	Authenticate(token string) (serverusers.User, error)
}

type serverUserContextKey struct{}

func WithServerGatewayAuth(store ServerGatewayUserStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := bearerToken(r.Header.Get("Authorization"))
		if token == "" {
			token = strings.TrimSpace(r.Header.Get("X-AI-Gate-Token"))
		}
		if token == "" || store == nil {
			http.Error(w, "valid AI Gate token is required", http.StatusUnauthorized)
			return
		}
		user, err := store.Authenticate(token)
		if err != nil {
			http.Error(w, "valid AI Gate token is required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), serverUserContextKey{}, user)))
	})
}

func ServerUserFromContext(ctx context.Context) (serverusers.User, bool) {
	user, ok := ctx.Value(serverUserContextKey{}).(serverusers.User)
	return user, ok
}

func bearerToken(header string) string {
	fields := strings.Fields(strings.TrimSpace(header))
	if len(fields) != 2 || !strings.EqualFold(fields[0], "bearer") {
		return ""
	}
	return fields[1]
}
