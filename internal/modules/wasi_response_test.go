package modules

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestDecodeWASIInvocationResponse(t *testing.T) {
	body := []byte("hello")
	valid := fmt.Sprintf(`{"schema_version":1,"status_code":207,"body":"%s"}`, base64.StdEncoding.EncodeToString(body))

	tests := []struct {
		name       string
		output     string
		maxBody    int
		wantStatus int
		wantBody   string
		wantErr    error
	}{
		{name: "empty legacy success", output: "", maxBody: 32, wantStatus: 200},
		{name: "structured success", output: valid, maxBody: 32, wantStatus: 207, wantBody: "hello"},
		{name: "malformed json", output: `{`, maxBody: 32, wantErr: ErrRuntimeResponseInvalid},
		{name: "unknown field", output: `{"schema_version":1,"status_code":200,"extra":true}`, maxBody: 32, wantErr: ErrRuntimeResponseInvalid},
		{name: "trailing json", output: `{"schema_version":1,"status_code":200}{}`, maxBody: 32, wantErr: ErrRuntimeResponseInvalid},
		{name: "wrong schema", output: `{"schema_version":2,"status_code":200}`, maxBody: 32, wantErr: ErrRuntimeResponseInvalid},
		{name: "informational status denied", output: `{"schema_version":1,"status_code":199}`, maxBody: 32, wantErr: ErrRuntimeResponseInvalid},
		{name: "status too high", output: `{"schema_version":1,"status_code":600}`, maxBody: 32, wantErr: ErrRuntimeResponseInvalid},
		{name: "invalid host bound", output: valid, maxBody: 0, wantErr: ErrRuntimeResponseInvalid},
		{name: "body too large", output: fmt.Sprintf(`{"schema_version":1,"status_code":200,"body":"%s"}`, base64.StdEncoding.EncodeToString([]byte("12345"))), maxBody: 4, wantErr: ErrRuntimeOutputTooLarge},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := decodeWASIInvocationResponse([]byte(tc.output), tc.maxBody)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("error = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if result.StatusCode != tc.wantStatus {
				t.Fatalf("status = %d, want %d", result.StatusCode, tc.wantStatus)
			}
			if string(result.Body) != tc.wantBody {
				t.Fatalf("body = %q, want %q", string(result.Body), tc.wantBody)
			}
		})
	}
}

func TestDecodeWASIInvocationResponseCopiesBody(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString([]byte("copy-me"))
	output := []byte(fmt.Sprintf(`{"schema_version":1,"status_code":200,"body":"%s"}`, encoded))
	result, err := decodeWASIInvocationResponse(output, 32)
	if err != nil {
		t.Fatal(err)
	}
	for i := range output {
		output[i] = 'x'
	}
	if strings.Contains(string(result.Body), "x") || string(result.Body) != "copy-me" {
		t.Fatalf("decoded body changed after source mutation: %q", result.Body)
	}
}
