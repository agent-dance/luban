package engine

import (
	"context"
	"testing"
)

func TestCoreEngineShutdownNilReceiverIsNoopThroughEngineInterface(t *testing.T) {
	var core *CoreEngine
	var eng Engine = core

	if err := eng.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown() error = %v, want nil", err)
	}
}
