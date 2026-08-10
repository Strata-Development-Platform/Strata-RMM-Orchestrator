package platform

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestSmartGroupSyncConcurrentLifecycleIsIdempotentAndRestartable(t *testing.T) {
	logger := zap.NewNop()
	sg := NewSmartGroupSync(time.Hour, nil, logger)

	for iteration := 0; iteration < 100; iteration++ {
		var wg sync.WaitGroup
		for i := 0; i < 16; i++ {
			wg.Add(2)
			go func() {
				defer wg.Done()
				sg.Start(context.Background())
			}()
			go func() {
				defer wg.Done()
				sg.Stop()
			}()
		}
		wg.Wait()

		// Normalize the state before proving that a completed lifecycle can
		// start again with a fresh stop channel.
		sg.Stop()
		sg.Start(context.Background())
		if !sg.Running() {
			t.Fatalf("iteration %d: sync did not restart after Stop", iteration)
		}
		sg.Stop()
		if sg.Running() {
			t.Fatalf("iteration %d: sync still reports running after Stop", iteration)
		}
	}
}

func TestSmartGroupSyncSequentialRestartUsesFreshStopChannel(t *testing.T) {
	logger := zap.NewNop()
	sg := NewSmartGroupSync(time.Hour, nil, logger)

	sg.Start(context.Background())
	first := sg.stopCh
	sg.Stop()

	sg.Start(context.Background())
	second := sg.stopCh
	if first == second {
		t.Fatal("restart reused the previous stop channel")
	}
	if !sg.Running() {
		t.Fatal("sync should be running after restart")
	}
	sg.Stop()
}
