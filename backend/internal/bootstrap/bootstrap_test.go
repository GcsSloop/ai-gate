package bootstrap_test

import (
	"context"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
)

func TestNewApp(t *testing.T) {
	t.Parallel()

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr: "127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	if app == nil {
		t.Fatal("NewApp returned nil app")
	}
}
