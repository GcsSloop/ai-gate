package refresh

import (
	"testing"
	"time"
)

func TestNewOrchestratorDefaultsToFifteenSecondTimeout(t *testing.T) {
	t.Parallel()

	orchestrator := NewOrchestrator(nil, nil, nil)
	if orchestrator.timeout != 15*time.Second {
		t.Fatalf("timeout = %s, want %s", orchestrator.timeout, 15*time.Second)
	}
}
