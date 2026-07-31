package comms

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/agent/core"
)

func TestEncodeMetricsUsesUnixTimestamp(t *testing.T) {
	wantTime := time.Unix(1_700_000_000, 0).UTC()
	payload := encodeMetrics([]core.MetricSample{{
		Name: "cpu.usage", Value: 42.5, Timestamp: wantTime,
	}})
	var decoded struct {
		Samples []struct {
			Name      string  `json:"name"`
			Value     float64 `json:"value"`
			Timestamp int64   `json:"timestamp"`
		} `json:"samples"`
	}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Samples) != 1 || decoded.Samples[0].Timestamp != wantTime.Unix() {
		t.Fatalf("wire payload = %s", payload)
	}
}
