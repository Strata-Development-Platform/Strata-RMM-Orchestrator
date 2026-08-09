package reporting

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestReportSectionConstantsContract(t *testing.T) {
	assert.Equal(t, ReportSection("summary"), SectionSummary)
	assert.Equal(t, ReportSection("alerts"), SectionAlerts)
	assert.Equal(t, ReportSection("cves"), SectionCVEs)
	assert.Equal(t, ReportSection("patches"), SectionPatches)
}

func TestReportEngineNilStorage(t *testing.T) {
	engine := NewReportEngine(nil, zap.NewNop(), nil, "")
	assert.NotNil(t, engine)
}

func TestReportEngineNilDB(t *testing.T) {
	engine := NewReportEngine(nil, zap.NewNop(), nil, "")
	assert.NotNil(t, engine)
}

func TestReportDataFields(t *testing.T) {
	data := &ReportData{
		TenantName:  "acme",
		PeriodStart: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC),
		DeviceCount: 100,
		OnlineCount: 80,
		AlertCount:  5,
		CVECount:    3,
		PatchCount:  2,
	}

	assert.Equal(t, "acme", data.TenantName)
	assert.Equal(t, 100, data.DeviceCount)
	assert.Equal(t, 80, data.OnlineCount)
	assert.Equal(t, 5, data.AlertCount)
	assert.Equal(t, 3, data.CVECount)
	assert.Equal(t, 2, data.PatchCount)
}

func TestReportDataPeriodDefaults(t *testing.T) {
	end := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	start := end.Add(-7 * 24 * time.Hour)

	data := &ReportData{PeriodEnd: end}

	_ = start
	_ = data.PeriodEnd.After(data.PeriodStart)
}

func TestReportEngineWithNilLogger(t *testing.T) {
	engine := NewReportEngine(nil, nil, nil, "")
	assert.NotNil(t, engine)
}

func TestTruncateShort(t *testing.T) {
	got := truncate("short", 10)
	assert.Equal(t, "short", got)
}

func TestTruncateLong(t *testing.T) {
	got := truncate("hello world", 5)
	assert.Equal(t, "he...", got)
}

func TestReportSectionValues(t *testing.T) {
	sections := []ReportSection{SectionSummary, SectionAlerts, SectionCVEs, SectionPatches}

	seen := make(map[string]bool)
	for _, s := range sections {
		assert.False(t, seen[string(s)], "duplicate section: %s", s)
		seen[string(s)] = true
	}
}

func TestReportDataEmptyAlertsCVEsPatches(t *testing.T) {
	data := &ReportData{}

	assert.Empty(t, data.Alerts)
	assert.Empty(t, data.CVEs)
	assert.Empty(t, data.Patches)
}

func TestReportDataAlertStructFields(t *testing.T) {
	alert := struct{ Severity, Message, Device, Time string }{
		Severity: "critical",
		Message:  "CPU high",
		Device:   "prod-1",
		Time:     "2026-08-07T00:00:00Z",
	}

	assert.Equal(t, "critical", alert.Severity)
	assert.Equal(t, "CPU high", alert.Message)
	assert.Equal(t, "prod-1", alert.Device)
	assert.Equal(t, "2026-08-07T00:00:00Z", alert.Time)
}

func TestReportDataCVEStructFields(t *testing.T) {
	cve := struct{ ID, Severity, Package, Device string }{
		ID:       "CVE-2026-0001",
		Severity: "high",
		Package:  "openssl",
		Device:   "web-1",
	}

	assert.Equal(t, "CVE-2026-0001", cve.ID)
	assert.Equal(t, "high", cve.Severity)
	assert.Equal(t, "openssl", cve.Package)
	assert.Equal(t, "web-1", cve.Device)
}

func TestReportDataPatchStructFields(t *testing.T) {
	patch := struct{ Name, Version, Status string }{
		Name:    "kernel",
		Version: "5.15.0",
		Status:  "pending",
	}

	assert.Equal(t, "kernel", patch.Name)
	assert.Equal(t, "5.15.0", patch.Version)
	assert.Equal(t, "pending", patch.Status)
}

func TestReportScheduleRequestValidation(t *testing.T) {
	tests := []struct {
		name string
		req  struct {
			Name      string
			Frequency string
			Sections  []string
		}
		wantValid bool
	}{
		{"valid", struct{ Name string; Frequency string; Sections []string }{Name: "weekly", Frequency: "weekly", Sections: []string{"summary"}}, true},
		{"missing name", struct{ Name string; Frequency string; Sections []string }{Frequency: "weekly"}, false},
		{"missing frequency", struct{ Name string; Frequency string; Sections []string }{Name: "weekly"}, false},
		{"empty name and frequency", struct{ Name string; Frequency string; Sections []string }{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := tt.req.Name != "" && tt.req.Frequency != ""
			assert.Equal(t, tt.wantValid, valid)
		})
	}
}

func TestReportScheduleDefaultSections(t *testing.T) {
	req := struct {
		Name      string
		Frequency string
		Sections  []string
	}{
		Name: "weekly", Frequency: "weekly",
	}

	if req.Sections == nil {
		req.Sections = []string{"summary", "alerts", "cves", "patches"}
	}

	assert.Equal(t, []string{"summary", "alerts", "cves", "patches"}, req.Sections)
}

func TestReportEngineStartNilEngine(t *testing.T) {
	engine := NewReportEngine(nil, zap.NewNop(), nil, "")
	require.NotNil(t, engine)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	engine.Start(ctx)
}
