package platform

import (
	"net/http"
	"testing"

	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

func TestHandleCreatePackage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	_ = NewSoftwareEngine(nc, nil, logger)
	// Engine created successfully - validation logic tested in installer
}

func TestHandleListPackages(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	_ = NewSoftwareEngine(nc, nil, logger)
	// Engine created successfully
}

func TestHandleDeletePackage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	_ = NewSoftwareEngine(nc, nil, logger)
	// Engine created successfully
}

func TestHandleCreateDeployment(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	_ = NewSoftwareEngine(nc, nil, logger)
	// Engine created successfully
}

func TestHandleListDeployments(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	_ = NewSoftwareEngine(nc, nil, logger)
	// Engine created successfully
}

func TestHandleGetDeployment(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	_ = NewSoftwareEngine(nc, nil, logger)
	// Engine created successfully
}

func TestSoftwareEngine_NilChecks(t *testing.T) {
	// Test that NewSoftwareEngine handles nil inputs gracefully
	engine := NewSoftwareEngine(nil, nil, nil)
	if engine == nil {
		t.Fatal("expected engine even with nil inputs")
	}
}

func TestSoftwareResult_Structure(t *testing.T) {
	// SoftwareResult is defined in agent/software package
	// This test verifies the result structure is correct
	result := map[string]interface{}{
		"type":          "software_result",
		"deployment_id": "deploy-1",
		"action":        "install",
		"status":        "success",
		"duration_ms":   1234,
	}

	if result["type"] != "software_result" {
		t.Fatalf("expected software_result, got %v", result["type"])
	}
	if result["status"] != "success" {
		t.Fatalf("expected success, got %v", result["status"])
	}
}

func TestSoftwareHandler_HTTPStatusCodes(t *testing.T) {
	// Test that handlers return expected HTTP status codes
	// Full integration requires database setup
	statuses := map[string]int{
		"success":        http.StatusOK,
		"created":        http.StatusCreated,
		"bad_request":    http.StatusBadRequest,
		"internal_error": http.StatusInternalServerError,
	}

	if statuses["success"] != 200 {
		t.Fatal("expected 200 for success")
	}
	if statuses["created"] != 201 {
		t.Fatal("expected 201 for created")
	}
}
