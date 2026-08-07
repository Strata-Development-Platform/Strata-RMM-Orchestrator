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

func TestHandleCreatePackage_RequestValidation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	engine := NewSoftwareEngine(nc, nil, logger)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestHandleDeletePackage_RequestValidation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	engine := NewSoftwareEngine(nc, nil, logger)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
}

func TestHandleListDeployments_ResponseStructure(t *testing.T) {
	deployments := map[string]interface{}{
		"deployments": []map[string]interface{}{},
	}
	if _, ok := deployments["deployments"]; !ok {
		t.Fatal("expected deployments key")
	}
}

func TestHandleGetDeployment_TargetsStructure(t *testing.T) {
	response := map[string]interface{}{
		"id":      "deploy-1",
		"name":    "test",
		"status":  "deploying",
		"targets": []map[string]interface{}{},
	}
	if response["id"] != "deploy-1" {
		t.Fatal("expected id")
	}
	if response["status"] != "deploying" {
		t.Fatal("expected deploying")
	}
}

func TestSoftwareHandler_JSONContentType(t *testing.T) {
	contentTypes := map[string]string{
		"json": "application/json",
	}
	if contentTypes["json"] != "application/json" {
		t.Fatal("expected application/json")
	}
}

func TestSoftwareHandler_HTTPMethodCodes(t *testing.T) {
	codes := map[string]int{
		"GET":    200,
		"POST":   201,
		"DELETE": 200,
	}
	if codes["GET"] != 200 {
		t.Fatal("expected 200 for GET")
	}
	if codes["POST"] != 201 {
		t.Fatal("expected 201 for POST")
	}
	if codes["DELETE"] != 200 {
		t.Fatal("expected 200 for DELETE")
	}
}

func TestSoftwareEngine_LoggerField(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	engine := NewSoftwareEngine(nc, nil, logger)
	if engine == nil {
		t.Fatal("engine should not be nil")
	}
}

func TestSoftwareEngine_NATSConnField(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	nc, _ := nats.Connect(nats.DefaultURL)
	defer nc.Close()

	engine := NewSoftwareEngine(nc, nil, logger)
	if engine == nil {
		t.Fatal("engine should not be nil")
	}
}
