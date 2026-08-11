package modules

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const (
	BrokerOperationDevicesGet = "devices.get"
	maxBrokerDeviceIDBytes     = 128
)

// BrokerDevice is the minimal device projection exposed to an add-on. Scope is
// used only for host-side authorization and is never returned to guest code.
type BrokerDevice struct {
	ID       string
	Hostname string
	Status   string
	Scope    ResourceScope
}

// BrokerDeviceResolver must resolve device identity and ownership from
// authoritative platform storage. Implementations must not trust tenant/client/
// site identifiers supplied by guest input.
type BrokerDeviceResolver interface {
	ResolveBrokerDevice(context.Context, string) (BrokerDevice, error)
}

type brokerDeviceGetRequest struct {
	DeviceID string `json:"device_id"`
}

type brokerDeviceGetResponse struct {
	ID       string `json:"id"`
	Hostname string `json:"hostname"`
	Status   string `json:"status"`
}

// NewDeviceGetBrokerOperation returns the first concrete read-only broker
// capability. The operation is permanently bound to devices.read and verifies
// that the device resolved from guest input belongs to the exact trusted scope
// supplied by Strata host code before returning any data.
func NewDeviceGetBrokerOperation(resolver BrokerDeviceResolver) (BrokerOperation, error) {
	if resolver == nil {
		return BrokerOperation{}, errors.New("broker device resolver is required")
	}
	return BrokerOperation{
		Name:       BrokerOperationDevicesGet,
		Permission: "devices.read",
		Handler: func(ctx context.Context, _ InstalledModule, trustedScope ResourceScope, input []byte) ([]byte, error) {
			request, err := decodeBrokerDeviceGetRequest(input)
			if err != nil {
				return nil, err
			}
			device, err := resolver.ResolveBrokerDevice(ctx, request.DeviceID)
			if err != nil {
				return nil, fmt.Errorf("resolve broker device: %w", err)
			}
			if strings.TrimSpace(device.ID) != request.DeviceID {
				return nil, fmt.Errorf("%w: resolved device identity mismatch", ErrBrokerScopeInvalid)
			}
			if err := validateServiceScope(device.Scope.MSPID, device.Scope.ClientID, device.Scope.SiteID); err != nil {
				return nil, fmt.Errorf("%w: resolved device scope is invalid", ErrBrokerScopeInvalid)
			}
			if device.Scope != trustedScope {
				return nil, fmt.Errorf("%w: resolved device scope does not match trusted invocation scope", ErrBrokerScopeInvalid)
			}
			output, err := json.Marshal(brokerDeviceGetResponse{
				ID:       device.ID,
				Hostname: device.Hostname,
				Status:   device.Status,
			})
			if err != nil {
				return nil, fmt.Errorf("encode broker device response: %w", err)
			}
			return output, nil
		},
	}, nil
}

func decodeBrokerDeviceGetRequest(input []byte) (brokerDeviceGetRequest, error) {
	var request brokerDeviceGetRequest
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return brokerDeviceGetRequest{}, fmt.Errorf("invalid devices.get request: %w", err)
	}
	if err := ensureBrokerJSONEOF(decoder); err != nil {
		return brokerDeviceGetRequest{}, err
	}
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	if request.DeviceID == "" {
		return brokerDeviceGetRequest{}, errors.New("devices.get requires device_id")
	}
	if len(request.DeviceID) > maxBrokerDeviceIDBytes {
		return brokerDeviceGetRequest{}, errors.New("devices.get device_id exceeds limit")
	}
	return request, nil
}

func ensureBrokerJSONEOF(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("invalid devices.get trailing data: %w", err)
	}
	return errors.New("invalid devices.get request: multiple JSON values")
}
