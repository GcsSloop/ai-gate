package bootstrap_test

import (
	"context"
	"testing"

	"github.com/gcssloop/codex-router/backend/internal/bootstrap"
)

func TestNewApp(t *testing.T) {
	t.Parallel()

	app, err := bootstrap.NewApp(context.Background(), bootstrap.Config{
		ListenAddr:   "127.0.0.1:0",
		DatabasePath: t.TempDir() + "/router.sqlite",
	})
	if err != nil {
		t.Fatalf("NewApp returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = app.Close()
	})
	if app == nil {
		t.Fatal("NewApp returned nil app")
	}
}
