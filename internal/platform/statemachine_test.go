package platform

import "testing"

func TestJobStateTransitions(t *testing.T) {
	t.Parallel()

	allowed := [][2]string{
		{"pending", "queued"},
		{"pending", "cancelled"},
		{"queued", "dispatched"},
		{"dispatched", "running"},
		{"dispatched", "failed"},
		{"running", "succeeded"},
		{"running", "failed"},
	}
	for _, transition := range allowed {
		if err := TransitionJob(transition[0], transition[1]); err != nil {
			t.Errorf("expected %s -> %s to be allowed: %v", transition[0], transition[1], err)
		}
	}

	rejected := [][2]string{
		{"pending", "succeeded"},
		{"queued", "succeeded"},
		{"succeeded", "failed"},
		{"failed", "succeeded"},
		{"cancelled", "running"},
		{"expired", "queued"},
	}
	for _, transition := range rejected {
		if err := TransitionJob(transition[0], transition[1]); err == nil {
			t.Errorf("expected %s -> %s to be rejected", transition[0], transition[1])
		}
	}
}

func TestBackoffDurationIsBounded(t *testing.T) {
	t.Parallel()

	for attempt := 1; attempt <= 20; attempt++ {
		delay := backoffDuration(attempt)
		if delay < 30_000_000_000 {
			t.Fatalf("attempt %d returned a delay below 30 seconds: %s", attempt, delay)
		}
		if delay > 30*60_000_000_000 {
			t.Fatalf("attempt %d exceeded the 30 minute cap: %s", attempt, delay)
		}
	}
}
