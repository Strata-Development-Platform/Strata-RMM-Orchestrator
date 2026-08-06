package groups

import (
	"encoding/json"
	"testing"
	"time"
)

func TestFilterValidate(t *testing.T) {
	tests := []struct {
		name   string
		filter Filter
		wantErr bool
	}{
		{"valid eq", Filter{Field: "os", Op: OpEq, Value: "linux"}, false},
		{"valid gt", Filter{Field: "ram_total_mb", Op: OpGT, Value: 8192}, false},
		{"valid contains", Filter{Field: "hostname", Op: OpContains, Value: "prod"}, false},
		{"valid in", Filter{Field: "status", Op: OpIn, Value: []interface{}{"online", "pending"}}, false},
		{"valid is_null", Filter{Field: "public_ip", Op: OpIsNull, Value: nil}, false},
		{"valid not_null", Filter{Field: "tags", Op: OpNotNull, Value: nil}, false},
		{"valid regex", Filter{Field: "hostname", Op: OpRegex, Value: "web-.*"}, false},
		{"empty field", Filter{Field: "", Op: OpEq, Value: "test"}, true},
		{"invalid operator", Filter{Field: "os", Op: "bogus", Value: "linux"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.filter.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Filter.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExpressionValidate(t *testing.T) {
	tests := []struct {
		name    string
		expr    Expression
		wantErr bool
	}{
		{"valid and", Expression{Condition: "AND", Filters: []Filter{{Field: "os", Op: OpEq, Value: "linux"}}}, false},
		{"valid or", Expression{Condition: "OR", Filters: []Filter{{Field: "status", Op: OpEq, Value: "online"}}}, false},
		{"valid nested", Expression{Condition: "AND", Filters: []Filter{{Field: "os", Op: OpEq, Value: "linux"}}, Nested: &Expression{Condition: "OR", Filters: []Filter{{Field: "status", Op: OpEq, Value: "online"}, {Field: "status", Op: OpEq, Value: "pending"}}}}, false},
		{"empty condition", Expression{Condition: "", Filters: []Filter{{Field: "os", Op: OpEq, Value: "linux"}}}, true},
		{"invalid condition", Expression{Condition: "XOR", Filters: []Filter{{Field: "os", Op: OpEq, Value: "linux"}}}, true},
		{"empty filters no nested", Expression{Condition: "AND", Filters: nil}, true},
		{"nested invalid", Expression{Condition: "AND", Filters: []Filter{{Field: "os", Op: OpEq, Value: "linux"}}, Nested: &Expression{Condition: "BAD", Filters: nil}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.expr.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Expression.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestEvaluate_Equality(t *testing.T) {
	dev := Device{
		Hostname: "web-server-01",
		OS:       "linux",
		OSVersion: "ubuntu-22.04",
		Arch:     "amd64",
		Status:   "online",
	}

	tests := []struct {
		name   string
		filter Filter
		dev    Device
		want   bool
	}{
		{"eq exact match", Filter{Field: "os", Op: OpEq, Value: "linux"}, dev, true},
		{"eq case insensitive", Filter{Field: "os", Op: OpEq, Value: "LINUX"}, dev, true},
		{"eq no match", Filter{Field: "os", Op: OpEq, Value: "windows"}, dev, false},
		{"neq not equal", Filter{Field: "os", Op: OpNeq, Value: "windows"}, dev, true},
		{"neq equal returns false", Filter{Field: "os", Op: OpNeq, Value: "linux"}, dev, false},
		{"eq hostname match", Filter{Field: "hostname", Op: OpEq, Value: "web-server-01"}, dev, true},
		{"eq arch match", Filter{Field: "arch", Op: OpEq, Value: "amd64"}, dev, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateFilter(tt.filter, tt.dev)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.want {
				t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter.Op, tt.filter.Field, result, tt.want)
			}
		})
	}
}

func TestEvaluate_StringOps(t *testing.T) {
	dev := Device{Hostname: "web-server-01-prod", OS: "linux", Status: "online"}

	tests := []struct {
		name   string
		filter Filter
		dev    Device
		want   bool
	}{
		{"contains match", Filter{Field: "hostname", Op: OpContains, Value: "server"}, dev, true},
		{"contains no match", Filter{Field: "hostname", Op: OpContains, Value: "db"}, dev, false},
		{"contains case insensitive", Filter{Field: "hostname", Op: OpContains, Value: "SERVER"}, dev, true},
		{"startswith match", Filter{Field: "hostname", Op: OpStartsWith, Value: "web"}, dev, true},
		{"startswith no match", Filter{Field: "hostname", Op: OpStartsWith, Value: "db"}, dev, false},
		{"startswith case insensitive", Filter{Field: "hostname", Op: OpStartsWith, Value: "WEB"}, dev, true},
		{"regex match", Filter{Field: "hostname", Op: OpRegex, Value: "^web-.*-prod$"}, dev, true},
		{"regex no match", Filter{Field: "hostname", Op: OpRegex, Value: "^db-.*"}, dev, false},
		{"invalid regex", Filter{Field: "hostname", Op: OpRegex, Value: "[invalid"}, dev, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateFilter(tt.filter, tt.dev)
			if err != nil && tt.name != "invalid regex" {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if tt.name == "invalid regex" && err == nil {
				t.Error("expected error for invalid regex")
				return
			}
			if result != tt.want {
				t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter.Op, tt.filter.Field, result, tt.want)
			}
		})
	}
}

func TestEvaluate_NumericOps(t *testing.T) {
	dev := Device{CPUCores: 8, RAMTotalMB: 16384, DiskTotalMB: 512000}

	tests := []struct {
		name   string
		filter Filter
		dev    Device
		want   bool
	}{
		{"gt exact", Filter{Field: "cpu_cores", Op: OpGT, Value: 4}, dev, true},
		{"gt not", Filter{Field: "cpu_cores", Op: OpGT, Value: 16}, dev, false},
		{"gte exact", Filter{Field: "cpu_cores", Op: OpGTE, Value: 8}, dev, true},
		{"gte less", Filter{Field: "cpu_cores", Op: OpGTE, Value: 16}, dev, false},
		{"lt exact", Filter{Field: "ram_total_mb", Op: OpLT, Value: 32768}, dev, true},
		{"lt not", Filter{Field: "ram_total_mb", Op: OpLT, Value: 8192}, dev, false},
		{"lte exact", Filter{Field: "ram_total_mb", Op: OpLTE, Value: 16384}, dev, true},
		{"lte greater", Filter{Field: "ram_total_mb", Op: OpLTE, Value: 8192}, dev, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateFilter(tt.filter, tt.dev)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.want {
				t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter.Op, tt.filter.Field, result, tt.want)
			}
		})
	}
}

func TestEvaluate_SetOps(t *testing.T) {
	dev := Device{
		Status:  "online",
		Tags:    []string{"production", "web", "critical"},
		PrivateIPs: []string{"10.0.1.5", "10.0.2.5"},
	}

	tests := []struct {
		name   string
		filter Filter
		dev    Device
		want   bool
	}{
		{"in match", Filter{Field: "status", Op: OpIn, Value: []interface{}{"online", "pending"}}, dev, true},
		{"in no match", Filter{Field: "status", Op: OpIn, Value: []interface{}{"offline", "disabled"}}, dev, false},
		{"contains_any match", Filter{Field: "tags", Op: OpContainsAny, Value: []interface{}{"production", "staging"}}, dev, true},
		{"contains_any no match", Filter{Field: "tags", Op: OpContainsAny, Value: []interface{}{"dev", "testing"}}, dev, false},
		{"contains_any array match", Filter{Field: "private_ips", Op: OpContainsAny, Value: []interface{}{"10.0.1.5", "192.168.1.1"}}, dev, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateFilter(tt.filter, tt.dev)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.want {
				t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter.Op, tt.filter.Field, result, tt.want)
			}
		})
	}
}

func TestEvaluate_NullOps(t *testing.T) {
	heartbeat := time.Now().Add(-time.Hour)
	none := time.Now().Add(24 * time.Hour)

	tests := []struct {
		name   string
		filter Filter
		dev    Device
		want   bool
	}{
		{"is_null empty string is not null", Filter{Field: "public_ip", Op: OpIsNull, Value: nil}, Device{PublicIP: ""}, false},
		{"is_null nil pointer", Filter{Field: "public_ip", Op: OpIsNull, Value: nil}, Device{PublicIP: ""}, false},
		{"is_null absent", Filter{Field: "last_heartbeat", Op: OpIsNull, Value: nil}, Device{LastHeartbeat: &none}, false},
		{"not_null present", Filter{Field: "last_heartbeat", Op: OpNotNull, Value: nil}, Device{LastHeartbeat: &heartbeat}, true},
		{"not_null empty string", Filter{Field: "public_ip", Op: OpNotNull, Value: nil}, Device{PublicIP: ""}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateFilter(tt.filter, tt.dev)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.want {
				t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter.Op, tt.filter.Field, result, tt.want)
			}
		})
	}
}

func TestEvaluate_TimeComparison(t *testing.T) {
	heartbeat := time.Now().Add(-time.Hour)
	tests := []struct {
		name   string
		filter Filter
		dev    Device
		want   bool
	}{
		{"lt time", Filter{Field: "last_heartbeat", Op: OpLT, Value: time.Now()}, Device{LastHeartbeat: &heartbeat}, true},
		{"gt time", Filter{Field: "last_heartbeat", Op: OpGT, Value: time.Now().Add(-2 * time.Hour)}, Device{LastHeartbeat: &heartbeat}, true},
		{"gte time same", Filter{Field: "last_heartbeat", Op: OpGTE, Value: heartbeat}, Device{LastHeartbeat: &heartbeat}, true},
		{"lte time past", Filter{Field: "last_heartbeat", Op: OpLTE, Value: time.Now()}, Device{LastHeartbeat: &heartbeat}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateFilter(tt.filter, tt.dev)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if result != tt.want {
				t.Errorf("evaluateFilter(%s, %s) = %v, want %v", tt.filter.Op, tt.filter.Field, result, tt.want)
			}
		})
	}
}

func TestEvaluate_AndExpression(t *testing.T) {
	dev := Device{OS: "linux", Status: "online", CPUCores: 8, RAMTotalMB: 16384}

	expr := Expression{
		Condition: "AND",
		Filters: []Filter{
			{Field: "os", Op: OpEq, Value: "linux"},
			{Field: "status", Op: OpEq, Value: "online"},
			{Field: "cpu_cores", Op: OpGT, Value: 4},
		},
	}
	got, err := expr.Evaluate(dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected device to match AND expression, got false")
	}
}

func TestEvaluate_AndExpression_Fail(t *testing.T) {
	dev := Device{OS: "linux", Status: "offline", CPUCores: 8, RAMTotalMB: 16384}

	expr := Expression{
		Condition: "AND",
		Filters: []Filter{
			{Field: "os", Op: OpEq, Value: "linux"},
			{Field: "status", Op: OpEq, Value: "online"},
		},
	}
	got, err := expr.Evaluate(dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected device to NOT match AND expression (status mismatch)")
	}
}

func TestEvaluate_OrExpression(t *testing.T) {
	dev := Device{OS: "windows", Status: "online", CPUCores: 4}

	expr := Expression{
		Condition: "OR",
		Filters: []Filter{
			{Field: "os", Op: OpEq, Value: "linux"},
			{Field: "os", Op: OpEq, Value: "windows"},
		},
	}
	got, err := expr.Evaluate(dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Error("expected device to match OR expression (windows match)")
	}
}

func TestEvaluate_OrExpression_Fail(t *testing.T) {
	dev := Device{OS: "macos", Status: "offline", CPUCores: 2}

	expr := Expression{
		Condition: "OR",
		Filters: []Filter{
			{Field: "os", Op: OpEq, Value: "linux"},
			{Field: "os", Op: OpEq, Value: "windows"},
		},
	}
	got, err := expr.Evaluate(dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected device to NOT match OR expression")
	}
}

func TestEvaluate_NestedExpression(t *testing.T) {
	// OR at top: matches if (OS==linux) OR (cpu_cores >= 16 AND status==online)
	dev1 := Device{OS: "linux", Status: "offline", CPUCores: 2}
	dev2 := Device{OS: "windows", Status: "online", CPUCores: 32}
	dev3 := Device{OS: "macos", Status: "online", CPUCores: 2}

	expr := Expression{
		Condition: "OR",
		Filters:   []Filter{{Field: "os", Op: OpEq, Value: "linux"}},
		Nested: &Expression{
			Condition: "AND",
			Filters: []Filter{
				{Field: "cpu_cores", Op: OpGTE, Value: 16},
				{Field: "status", Op: OpEq, Value: "online"},
			},
		},
	}

	tests := []struct {
		name string
		dev  Device
		want bool
	}{
		{"linux offline low cpu", dev1, true},
		{"windows online high cpu", dev2, true},
		{"macos online low cpu", dev3, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expr.Evaluate(tt.dev)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("nested expr evaluate = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEvaluate_EmptyDevice(t *testing.T) {
	dev := Device{}

	expr := Expression{
		Condition: "AND",
		Filters: []Filter{
			{Field: "status", Op: OpEq, Value: "online"},
		},
	}
	got, err := expr.Evaluate(dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected empty device to not match status filter")
	}
}

func TestEvaluate_FieldNotInDevice(t *testing.T) {
	dev := Device{OS: "linux", Status: "online"}

	expr := Expression{
		Condition: "AND",
		Filters: []Filter{
			{Field: "nonexistent", Op: OpEq, Value: "test"},
		},
	}
	got, err := expr.Evaluate(dev)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got {
		t.Error("expected device to not match unknown field filter")
	}
}

func TestParseExpression(t *testing.T) {
	data := []byte(`{"condition":"AND","filters":[{"field":"os","op":"eq","value":"linux"},{"field":"status","op":"eq","value":"online"}]}`)
	expr, err := ParseExpression(data)
	if err != nil {
		t.Fatalf("ParseExpression error: %v", err)
	}
	if expr.Condition != "AND" {
		t.Errorf("expected condition AND, got %s", expr.Condition)
	}
	if len(expr.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(expr.Filters))
	}
	if expr.Filters[0].Field != "os" {
		t.Errorf("expected first filter field os, got %s", expr.Filters[0].Field)
	}
	if expr.Filters[0].Op != OpEq {
		t.Errorf("expected first filter op eq, got %s", expr.Filters[0].Op)
	}
}

func TestParseExpression_Invalid(t *testing.T) {
	tests := [][]byte{
		[]byte(`{"condition":"XOR","filters":[]}`),
		[]byte(`{"condition":"AND","filters":[{"op":"eq","value":"test"}]}`),
		[]byte(`{"condition":"AND","filters":[]}`),
		[]byte(`not json`),
		[]byte(`{}`),
	}
	for i, data := range tests {
		t.Run("invalid_input_"+string(data[:min(len(data), 20)]), func(t *testing.T) {
			_, err := ParseExpression(data)
			if err == nil {
				t.Errorf("expected ParseExpression to fail for input %s", data)
			}
			_ = i
		})
	}
}

func TestSerializeExpression(t *testing.T) {
	expr := Expression{
		Condition: "AND",
		Filters: []Filter{
			{Field: "os", Op: OpEq, Value: "linux"},
			{Field: "status", Op: OpEq, Value: "online"},
		},
	}
	data, err := SerializeExpression(expr)
	if err != nil {
		t.Fatalf("SerializeExpression error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty serialized data")
	}
	// Round-trip
	parsed, err := ParseExpression(data)
	if err != nil {
		t.Fatalf("round-trip ParseExpression error: %v", err)
	}
	if parsed.Condition != expr.Condition {
		t.Errorf("round-trip condition mismatch: got %s, want %s", parsed.Condition, expr.Condition)
	}
	if len(parsed.Filters) != len(expr.Filters) {
		t.Errorf("round-trip filter count mismatch: got %d, want %d", len(parsed.Filters), len(expr.Filters))
	}
}

func TestIsSmartGroup(t *testing.T) {
	tests := []struct {
			name       string
			raw        json.RawMessage
			wantSmart  bool
		}{
			{"empty", json.RawMessage(`{}`), false},
			{"null", json.RawMessage(`null`), false},
			{"empty string", json.RawMessage(`""`), false},
			{"has filters", json.RawMessage(`{"condition":"AND","filters":[{"field":"os","op":"eq","value":"linux"}]}`), true},
			{"has nested", json.RawMessage(`{"condition":"AND","nested":{"condition":"OR","filters":[{"field":"status","op":"eq","value":"online"}]}}`), true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := IsSmartGroup(tt.raw)
				if got != tt.wantSmart {
					t.Errorf("IsSmartGroup() = %v, want %v", got, tt.wantSmart)
				}
			})
		}
	}

func TestValuesEqual(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		want bool
	}{
		{"string equal", "linux", "linux", true},
		{"string case insensitive", "Linux", "LINUX", true},
		{"string not equal", "linux", "windows", false},
		{"int equal", 8, 8, true},
		{"int vs float64", 8, 8.0, true},
		{"int64 vs float64", int64(16384), 16384.0, true},
		{"bool equal", true, true, true},
		{"bool not equal", true, false, false},
		{"nil both", nil, nil, true},
		{"nil vs string", nil, "test", false},
		{"time equal", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := valuesEqual(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("valuesEqual(%v, %v) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCompareNumeric(t *testing.T) {
	tests := []struct {
		name string
		a    interface{}
		b    interface{}
		op   FilterOperator
		want bool
	}{
		{"int gt", 8, 4, OpGT, true},
		{"int64 gte", int64(16384), int64(16384), OpGTE, true},
		{"float64 lt", 100.5, 200.0, OpLT, true},
		{"int lte", 4, 8, OpLTE, true},
		{"string numeric parse", "100", "50", OpGT, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := compareNumeric(tt.a, tt.b, tt.op)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tt.want {
				t.Errorf("compareNumeric(%v, %v, %s) = %v, want %v", tt.a, tt.b, tt.op, got, tt.want)
			}
		})
	}
}

func TestGetDeviceValue(t *testing.T) {
	heartbeat := time.Now()
	dev := Device{
		Hostname:      "test",
		OS:            "linux",
		Arch:          "amd64",
		CPUCores:      8,
		RAMTotalMB:    16384,
		Status:        "online",
		LastHeartbeat: &heartbeat,
		CreatedAt:     time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Tags:          []string{"prod"},
	}

	tests := []struct {
		field  string
		wantOK bool
	}{
		{"hostname", true},
		{"os", true},
		{"arch", true},
		{"cpu_cores", true},
		{"ram_total_mb", true},
		{"status", true},
		{"last_heartbeat", true},
		{"created_at", true},
		{"tags", true},
		{"public_ip", true},
		{"nonexistent", false},
	}
	for _, tt := range tests {
		t.Run(tt.field, func(t *testing.T) {
			got := getDeviceValue(tt.field, dev)
			if tt.wantOK && got == nil {
				t.Errorf("getDeviceValue(%s) = nil, expected non-nil", tt.field)
			}
			if !tt.wantOK && got != nil {
				t.Errorf("getDeviceValue(%s) = %v, expected nil", tt.field, got)
			}
		})
	}
}

func TestEvaluate_LastSeenMinutes(t *testing.T) {
	// Simulate last_heartbeat check: device must have heartbeated within last 30 minutes
	now := time.Now()
	recent := now.Add(-10 * time.Minute)
	stale := now.Add(-2 * time.Hour)

	tests := []struct {
		name   string
		hb     *time.Time
		filter Filter
		want   bool
	}{
		{"recent heartbeat lt now", &recent, Filter{Field: "last_heartbeat", Op: OpLT, Value: now}, true},
		{"stale heartbeat lt now", &stale, Filter{Field: "last_heartbeat", Op: OpLT, Value: now}, true},
		{"recent heartbeat gt 2h_ago", &recent, Filter{Field: "last_heartbeat", Op: OpGT, Value: now.Add(-2 * time.Hour)}, true},
		{"stale heartbeat lt 15m_ago", &stale, Filter{Field: "last_heartbeat", Op: OpLT, Value: now.Add(-15 * time.Minute)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dev := Device{OS: "linux", Status: "online", LastHeartbeat: tt.hb}
			result, err := evaluateFilter(tt.filter, dev)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result != tt.want {
				t.Errorf("evaluateFilter = %v, want %v", result, tt.want)
			}
		})
	}
}
