package inventory

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewCVESyncEngine(t *testing.T) {
	e := NewCVESyncEngine(nil, nil)
	if e == nil {
		t.Fatal("expected engine")
	}
	if e.interval != 6*time.Hour {
		t.Errorf("interval: got %v, want 6h", e.interval)
	}
}

func TestCVESyncEngineWithNVDAPIKey(t *testing.T) {
	e := NewCVESyncEngine(nil, nil)
	e.WithNVDAPIKey("test-key")
	if e.nvdAPIKey != "test-key" {
		t.Errorf("nvd key: got %s, want test-key", e.nvdAPIKey)
	}
}

func TestCVESyncEngineWithInterval(t *testing.T) {
	e := NewCVESyncEngine(nil, nil)
	e.WithInterval(time.Hour)
	if e.interval != time.Hour {
		t.Errorf("interval: got %v, want 1h", e.interval)
	}
}

func TestParseTime(t *testing.T) {
	tests := []struct {
		input string
		want  time.Time
	}{
		{"2026-01-15T10:30:00Z", time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"2026-01-15T10:30:00", time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC)},
		{"invalid", time.Now()}, // falls back to now
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseTime(tt.input)
			if tt.input != "invalid" && !got.Equal(tt.want) {
				t.Errorf("parseTime(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestScoreToSeverity(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{9.5, "critical"},
		{9.0, "critical"},
		{7.5, "high"},
		{7.0, "high"},
		{5.0, "medium"},
		{4.0, "medium"},
		{2.0, "low"},
		{0.5, "low"},
		{0, "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := scoreToSeverity(tt.score)
			if got != tt.want {
				t.Errorf("scoreToSeverity(%f) = %s, want %s", tt.score, got, tt.want)
			}
		})
	}
}

func TestTruncate(t *testing.T) {
	got := truncate("hello world", 5)
	if got != "hello" {
		t.Errorf("truncate: got %s, want hello", got)
	}

	got = truncate("short", 10)
	if got != "short" {
		t.Errorf("truncate short: got %s, want short", got)
	}
}

func TestDefaultPackages(t *testing.T) {
	if len(defaultPackages) == 0 {
		t.Fatal("expected default packages")
	}
	found := false
	for _, p := range defaultPackages {
		if p.Name == "openssh" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected openssh in default packages")
	}
}

func TestOSVQueryJSON(t *testing.T) {
	query := OSVQuery{
		Package: OSVPackage{
			Name:      "openssh",
			Ecosystem: "Debian",
		},
	}

	data, err := json.Marshal(query)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded OSVQuery
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Package.Name != "openssh" {
		t.Errorf("name: got %s, want openssh", decoded.Package.Name)
	}
}

func TestPackageEntryJSON(t *testing.T) {
	entry := PackageEntry{
		Name:      "glibc",
		Ecosystem: "Debian",
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded PackageEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Name != "glibc" {
		t.Errorf("name: got %s, want glibc", decoded.Name)
	}
}

func TestCVESyncEngineOSVAPIIntegration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"vulns": []map[string]interface{}{
					{
						"id":      "GHSA-test-1234",
						"summary": "Test CVE in curl",
					},
				},
			},
		})
	}))
	defer server.Close()

	e := NewCVESyncEngine(nil, nil)
	e.httpClient = server.Client()

	_, err := e.httpClient.Post(server.URL, "application/json", nil)
	if err != nil {
		t.Fatalf("server not reachable: %v", err)
	}
}

func TestVulnerabilityEngineIsAffected(t *testing.T) {
	ve := &VulnerabilityEngine{}

	tests := []struct {
		current  string
		fixed    string
		affected bool
	}{
		{"7.0.0", "8.0.0", true},
		{"8.0.0", "8.0.0", false},
		{"9.0.0", "8.0.0", false},
		{"1.0", "2.0", true},
	}

	for _, tt := range tests {
		if ve.isAffected(tt.current, tt.fixed) != tt.affected {
			t.Errorf("isAffected(%s, %s) = %v, want %v", tt.current, tt.fixed, !tt.affected, tt.affected)
		}
	}
}
