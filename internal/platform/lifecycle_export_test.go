package platform

import "testing"

func TestLifecycleExportLimit(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		want      int
		wantError bool
	}{
		{name: "default", want: defaultLifecycleExportLimit},
		{name: "one record", raw: "1", want: 1},
		{name: "maximum", raw: "5000", want: maxLifecycleExportLimit},
		{name: "zero", raw: "0", wantError: true},
		{name: "above maximum", raw: "5001", wantError: true},
		{name: "invalid", raw: "all", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, validationError := lifecycleExportLimit(tt.raw)
			if (validationError != "") != tt.wantError {
				t.Fatalf("validation error = %q, wantError %v", validationError, tt.wantError)
			}
			if !tt.wantError && got != tt.want {
				t.Fatalf("limit = %d, want %d", got, tt.want)
			}
		})
	}
}
