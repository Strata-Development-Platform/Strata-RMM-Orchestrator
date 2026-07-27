package reporting

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jung-kurt/gofpdf/v2"
	"go.uber.org/zap"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

type ReportEngine struct {
	db      *sql.DB
	logger  *zap.Logger
	storage storage.Backend
	bucket  string
}

func NewReportEngine(db *sql.DB, logger *zap.Logger, storage storage.Backend, bucket string) *ReportEngine {
	return &ReportEngine{
		db:      db,
		logger:  logger,
		storage: storage,
		bucket:  bucket,
	}
}

type ReportSection string

const (
	SectionSummary ReportSection = "summary"
	SectionAlerts  ReportSection = "alerts"
	SectionCVEs    ReportSection = "cves"
	SectionPatches ReportSection = "patches"
)

type ReportData struct {
	TenantName  string
	PeriodStart time.Time
	PeriodEnd   time.Time
	DeviceCount int
	OnlineCount int
	AlertCount  int
	CVECount    int
	PatchCount  int
	Alerts      []struct{ Severity, Message, Device, Time string }
	CVEs        []struct{ ID, Severity, Package, Device string }
	Patches     []struct{ Name, Version, Status string }
}

func (e *ReportEngine) GenerateReport(ctx context.Context, tenantID string, sections []ReportSection, tenantName string) ([]byte, string, error) {
	data, err := e.gatherData(ctx, tenantID)
	if err != nil {
		return nil, "", fmt.Errorf("gather data: %w", err)
	}
	data.TenantName = tenantName
	data.PeriodEnd = time.Now()
	data.PeriodStart = data.PeriodEnd.Add(-7 * 24 * time.Hour)

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(20, 20, 20)
	pdf.AddPage()

	e.addHeader(pdf, data)
	e.addSummary(pdf, data)

	for _, s := range sections {
		switch s {
		case SectionAlerts:
			e.addAlerts(pdf, data)
		case SectionCVEs:
			e.addCVEs(pdf, data)
		case SectionPatches:
			e.addPatches(pdf, data)
		}
	}

	e.addFooter(pdf)

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, "", fmt.Errorf("pdf output: %w", err)
	}

	key := fmt.Sprintf("reports/%s/%s.pdf", tenantID, time.Now().Format("2006-01-02-150405"))
	return buf.Bytes(), key, nil
}

func (e *ReportEngine) gatherData(ctx context.Context, tenantID string) (*ReportData, error) {
	data := &ReportData{}
	e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE tenant_id = $1`, tenantID).Scan(&data.DeviceCount)
	e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices WHERE tenant_id = $1 AND status = 'online'`, tenantID).Scan(&data.OnlineCount)
	e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM alerts WHERE tenant_id = $1 AND status = 'firing'`, tenantID).Scan(&data.AlertCount)
	e.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM device_vulnerabilities dv JOIN devices d ON dv.device_id = d.id WHERE d.tenant_id = $1 AND dv.status = 'open'`, tenantID).Scan(&data.CVECount)
	return data, nil
}

func (e *ReportEngine) addHeader(pdf *gofpdf.Fpdf, data *ReportData) {
	pdf.SetFont("Helvetica", "B", 24)
	pdf.Cell(0, 15, "Strata RMM Report")
	pdf.Ln(12)
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Customer: %s", data.TenantName))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Period: %s - %s", data.PeriodStart.Format("Jan 2, 2006"), data.PeriodEnd.Format("Jan 2, 2006")))
	pdf.Ln(5)
	pdf.Cell(0, 6, fmt.Sprintf("Generated: %s", time.Now().Format("Jan 2, 2006 15:04 MST")))
	pdf.Ln(10)
}

func (e *ReportEngine) addSummary(pdf *gofpdf.Fpdf, data *ReportData) {
	pdf.SetFont("Helvetica", "B", 14)
	pdf.Cell(0, 10, "Executive Summary")
	pdf.Ln(10)

	pdf.SetFont("Helvetica", "", 10)
	colW := 45.0
	x := pdf.GetX()

	for _, s := range []struct {
		label string
		value int
		color []int
	}{
		{"Total Devices", data.DeviceCount, []int{52, 73, 94}},
		{"Online", data.OnlineCount, []int{46, 204, 113}},
		{"Active Alerts", data.AlertCount, []int{231, 76, 60}},
		{"Open CVEs", data.CVECount, []int{230, 126, 34}},
	} {
		pdf.SetDrawColor(200, 200, 200)
		pdf.SetFillColor(s.color[0], s.color[1], s.color[2])
		pdf.Rect(x, pdf.GetY(), colW, 18, "F")
		pdf.SetTextColor(255, 255, 255)
		pdf.SetFont("Helvetica", "B", 16)
		pdf.Text(x+5, pdf.GetY()+8, fmt.Sprintf("%d", s.value))
		pdf.SetFont("Helvetica", "", 7)
		pdf.Text(x+5, pdf.GetY()+13, s.label)
		pdf.SetTextColor(0, 0, 0)
		x += colW + 3
	}
	pdf.Ln(22)
}

func (e *ReportEngine) addAlerts(pdf *gofpdf.Fpdf, data *ReportData) {
	pdf.SetFont("Helvetica", "B", 14)
	pdf.Cell(0, 10, "Active Alerts")
	pdf.Ln(10)

	if data.AlertCount == 0 {
		pdf.SetFont("Helvetica", "", 10)
		pdf.Cell(0, 6, "No active alerts")
		pdf.Ln(8)
		return
	}

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(240, 240, 240)
	pdf.CellFormat(15, 6, "Sev", "1", 0, "C", true, 0, "")
	pdf.CellFormat(80, 6, "Message", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 6, "Device", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 6, "Time", "1", 0, "L", true, 0, "")
	pdf.Ln(6)

	pdf.SetFont("Helvetica", "", 7)
	for _, a := range data.Alerts {
		pdf.CellFormat(15, 5, a.Severity, "1", 0, "C", false, 0, "")
		pdf.CellFormat(80, 5, truncate(a.Message, 55), "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 5, truncate(a.Device, 20), "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 5, a.Time, "1", 0, "L", false, 0, "")
		pdf.Ln(6)
	}
	pdf.Ln(5)
}

func (e *ReportEngine) addCVEs(pdf *gofpdf.Fpdf, data *ReportData) {
	pdf.SetFont("Helvetica", "B", 14)
	pdf.Cell(0, 10, "Open Vulnerabilities")
	pdf.Ln(10)

	if data.CVECount == 0 {
		pdf.SetFont("Helvetica", "", 10)
		pdf.Cell(0, 6, "No open vulnerabilities")
		pdf.Ln(8)
		return
	}

	pdf.SetFont("Helvetica", "B", 8)
	pdf.SetFillColor(240, 240, 240)
	pdf.CellFormat(30, 6, "CVE ID", "1", 0, "C", true, 0, "")
	pdf.CellFormat(15, 6, "Sev", "1", 0, "C", true, 0, "")
	pdf.CellFormat(60, 6, "Package", "1", 0, "L", true, 0, "")
	pdf.CellFormat(40, 6, "Device", "1", 0, "L", true, 0, "")
	pdf.Ln(6)

	pdf.SetFont("Helvetica", "", 7)
	for _, c := range data.CVEs {
		pdf.CellFormat(30, 5, c.ID, "1", 0, "C", false, 0, "")
		pdf.CellFormat(15, 5, c.Severity, "1", 0, "C", false, 0, "")
		pdf.CellFormat(60, 5, c.Package, "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 5, truncate(c.Device, 20), "1", 0, "L", false, 0, "")
		pdf.Ln(5)
	}
	pdf.Ln(5)
}

func (e *ReportEngine) addPatches(pdf *gofpdf.Fpdf, data *ReportData) {
	pdf.SetFont("Helvetica", "B", 14)
	pdf.Cell(0, 10, "Patch Status")
	pdf.Ln(10)
	pdf.SetFont("Helvetica", "", 10)
	pdf.Cell(0, 6, fmt.Sprintf("Patches pending: %d", data.PatchCount))
	pdf.Ln(8)
}

func (e *ReportEngine) addFooter(pdf *gofpdf.Fpdf) {
	pdf.SetY(-30)
	pdf.SetFont("Helvetica", "I", 8)
	pdf.SetTextColor(128, 128, 128)
	pdf.Cell(0, 10, "Strata RMM - Generated by Strata Development Platform")
	pdf.Ln(4)
	pdf.Cell(0, 10, fmt.Sprintf("Page %d/{nb}", pdf.PageNo()))
}

func (e *ReportEngine) Start(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			e.processSchedules(ctx)
		case <-ctx.Done():
			return
		}
	}
}

func (e *ReportEngine) processSchedules(ctx context.Context) {
	rows, err := e.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, sections FROM report_schedules
		WHERE enabled = true
	`)
	if err != nil {
		e.logger.Error("query schedules", zap.Error(err))
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id, tenantID, name string
		var sectionsJSON []byte
		if err := rows.Scan(&id, &tenantID, &name, &sectionsJSON); err != nil {
			continue
		}
		go func(scheduleID, tid, n string, sections []byte) {
			ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
			defer cancel()

			var secs []ReportSection
			if err := json.Unmarshal(sections, &secs); err != nil {
				return
			}

			var tenantName string
			e.db.QueryRowContext(ctx, `SELECT name FROM tenants WHERE id = $1`, tid).Scan(&tenantName)

			pdfData, key, err := e.GenerateReport(ctx, tid, secs, tenantName)
			if err != nil {
				e.logger.Error("generate report", zap.Error(err))
				return
			}

			if e.storage != nil {
				_, err := e.storage.Upload(ctx, key, bytes.NewReader(pdfData), storage.UploadOptions{
					ContentType: "application/pdf",
				})
				if err != nil {
					e.logger.Error("upload report", zap.Error(err))
					return
				}
			}

			e.db.ExecContext(ctx, `
				INSERT INTO generated_reports (schedule_id, tenant_id, name, format, storage_key, size_bytes)
				VALUES ($1, $2, $3, 'pdf', $4, $5)
			`, scheduleID, tid, n, key, int64(len(pdfData)))

			e.db.ExecContext(ctx, `UPDATE report_schedules SET last_sent = NOW() WHERE id = $1`, scheduleID)

			e.logger.Info("report generated", zap.String("tenant", tid), zap.String("name", n))
		}(id, tenantID, name, sectionsJSON)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

var _ = gofpdf.New
