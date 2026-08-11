package modules

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type brokerDeviceResolverFunc func(context.Context, string) (BrokerDevice, error)

func (f brokerDeviceResolverFunc) ResolveBrokerDevice(ctx context.Context, id string) (BrokerDevice, error) {
	return f(ctx, id)
}

func TestDeviceGetBrokerOperationReturnsScopedDevice(t *testing.T) {
	trusted := ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-1"}
	operation, err := NewDeviceGetBrokerOperation(brokerDeviceResolverFunc(func(_ context.Context, id string) (BrokerDevice, error) {
		if id != "device-1" {
			t.Fatalf("device id = %q", id)
		}
		return BrokerDevice{ID: id, Hostname: "workstation-01", Status: "online", Scope: trusted}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	if operation.Name != BrokerOperationDevicesGet || operation.Permission != "devices.read" {
		t.Fatalf("unexpected operation: %+v", operation)
	}
	output, err := operation.Handler(context.Background(), InstalledModule{}, trusted, []byte(`{"device_id":"device-1"}`))
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	if got, want := string(output), `{"id":"device-1","hostname":"workstation-01","status":"online"}`; got != want {
		t.Fatalf("output = %s, want %s", got, want)
	}
}

func TestDeviceGetBrokerOperationRejectsSiblingScopeSubstitution(t *testing.T) {
	trusted := ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-1"}
	operation, err := NewDeviceGetBrokerOperation(brokerDeviceResolverFunc(func(context.Context, string) (BrokerDevice, error) {
		return BrokerDevice{
			ID:    "device-2",
			Scope: ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-2"},
		}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = operation.Handler(context.Background(), InstalledModule{}, trusted, []byte(`{"device_id":"device-2"}`))
	if !errors.Is(err, ErrBrokerScopeInvalid) {
		t.Fatalf("error = %v, want ErrBrokerScopeInvalid", err)
	}
}

func TestDeviceGetBrokerOperationRejectsResolvedIdentityMismatch(t *testing.T) {
	trusted := ResourceScope{MSPID: "msp-1"}
	operation, err := NewDeviceGetBrokerOperation(brokerDeviceResolverFunc(func(context.Context, string) (BrokerDevice, error) {
		return BrokerDevice{ID: "different-device", Scope: trusted}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = operation.Handler(context.Background(), InstalledModule{}, trusted, []byte(`{"device_id":"device-1"}`))
	if !errors.Is(err, ErrBrokerScopeInvalid) {
		t.Fatalf("error = %v, want ErrBrokerScopeInvalid", err)
	}
}

func TestDeviceGetBrokerOperationRejectsMalformedAndAmbiguousInput(t *testing.T) {
	operation, err := NewDeviceGetBrokerOperation(brokerDeviceResolverFunc(func(context.Context, string) (BrokerDevice, error) {
		t.Fatal("resolver must not be called")
		return BrokerDevice{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{
		`{}`,
		`{"device_id":"device-1","site_id":"site-2"}`,
		`{"device_id":"device-1"} {"device_id":"device-2"}`,
		`{"device_id":`,
		`{"device_id":"` + strings.Repeat("x", maxBrokerDeviceIDBytes+1) + `"}`,
	} {
		if _, err := operation.Handler(context.Background(), InstalledModule{}, ResourceScope{MSPID: "msp-1"}, []byte(input)); err == nil {
			t.Fatalf("input %q unexpectedly accepted", input)
		}
	}
}

func TestDeviceGetBrokerOperationPropagatesResolverFailureWithoutGuestData(t *testing.T) {
	backendErr := errors.New("database row lookup failed for tenant secret-42")
	operation, err := NewDeviceGetBrokerOperation(brokerDeviceResolverFunc(func(context.Context, string) (BrokerDevice, error) {
		return BrokerDevice{}, backendErr
	}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = operation.Handler(context.Background(), InstalledModule{}, ResourceScope{MSPID: "msp-1"}, []byte(`{"device_id":"device-1"}`))
	if !errors.Is(err, backendErr) {
		t.Fatalf("error = %v, want resolver error", err)
	}
	if status := brokerABIStatusForError(err); status != wasiBrokerStatusBackendFailure {
		t.Fatalf("status = %d, want backend failure", status)
	}
}

func TestNewDeviceGetBrokerOperationRequiresResolver(t *testing.T) {
	if _, err := NewDeviceGetBrokerOperation(nil); err == nil {
		t.Fatal("expected resolver validation error")
	}
}
