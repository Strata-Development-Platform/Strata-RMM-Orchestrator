package modules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const wasiResponseSchema = 1

var ErrRuntimeResponseInvalid = errors.New("module runtime response is invalid")

type wasiResponseEnvelope struct {
	SchemaVersion int    `json:"schema_version"`
	StatusCode    int    `json:"status_code"`
	Body          []byte `json:"body,omitempty"`
}

func decodeWASIInvocationResponse(output []byte, maxBodyBytes int) (InvocationResult, error) {
	if maxBodyBytes <= 0 {
		return InvocationResult{}, fmt.Errorf("%w: invalid host body limit", ErrRuntimeResponseInvalid)
	}
	trimmed := bytes.TrimSpace(output)
	if len(trimmed) == 0 {
		// Preserve the Alpha engine's original empty-stdout success behavior while
		// requiring every non-empty response to use the versioned response ABI.
		return InvocationResult{StatusCode: 200}, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	var response wasiResponseEnvelope
	if err := decoder.Decode(&response); err != nil {
		return InvocationResult{}, fmt.Errorf("%w: decode response", ErrRuntimeResponseInvalid)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return InvocationResult{}, fmt.Errorf("%w: trailing JSON", ErrRuntimeResponseInvalid)
	}
	if response.SchemaVersion != wasiResponseSchema {
		return InvocationResult{}, fmt.Errorf("%w: unsupported schema version", ErrRuntimeResponseInvalid)
	}
	if response.StatusCode < 200 || response.StatusCode > 599 {
		return InvocationResult{}, fmt.Errorf("%w: status code out of range", ErrRuntimeResponseInvalid)
	}
	if len(response.Body) > maxBodyBytes {
		return InvocationResult{}, ErrRuntimeOutputTooLarge
	}
	return InvocationResult{
		StatusCode: response.StatusCode,
		Body:       append([]byte(nil), response.Body...),
	}, nil
}
