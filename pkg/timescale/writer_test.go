//go:build dbintegration

package timescale

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestClientDBFor(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	// Test with write-only client (no replica)
	ctx := context.Background()
	writeOnlyClient, err := NewClient(ctx, "postgres://localhost:5432/write?sslmode=disable", "")
	if err != nil {
		t.Fatalf("failed to create write-only client: %v", err)
	}
	defer writeOnlyClient.Close()

	// DB() should return writeDB
	if writeOnlyClient.DB() == nil {
		t.Error("DB() should return writeDB")
	}

	// ReadDB() should return nil when no replica
	if writeOnlyClient.ReadDB() != nil {
		t.Error("ReadDB() should return nil when no replica configured")
	}

	// DBFor("write") should return writeDB
	if writeOnlyClient.DBFor("write") == nil {
		t.Error("DBFor(write) should return writeDB")
	}

	// DBFor("read") should return nil when no replica
	if writeOnlyClient.DBFor("read") != nil {
		t.Error("DBFor(read) should return nil when no replica configured")
	}
}

func TestClientDBForWithReplica(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	// Test with replica (note: this will fail to connect since no actual DB exists,
	// but we can still verify the routing logic by checking the struct)
	// We'll verify the logic by checking the code path

	// Create a write-only client first to ensure it works
	ctx := context.Background()
	client, err := NewClient(ctx, "postgres://localhost:5432/test?sslmode=disable", "postgres://localhost:5433/test?sslmode=disable")
	if err != nil {
		// If replica fails, we still have a valid write-only client
		// This is expected in CI without a replica DB
		if client == nil {
			t.Fatalf("NewClient should not fail when replica is unavailable: %v", err)
		}
		// Write-only client is valid, just skip replica tests
		t.Skip("Replica DB not available, skipping replica routing tests")
	}
	defer client.Close()

	// With replica, both should be non-nil
	if client.DB() == nil {
		t.Error("DB() should return writeDB")
	}
	if client.ReadDB() == nil {
		t.Error("ReadDB() should return replica when configured")
	}

	// DBFor("write") should return writeDB
	if client.DBFor("write") == nil {
		t.Error("DBFor(write) should return writeDB")
	}

	// DBFor("read") should return replica
	if client.DBFor("read") == nil {
		t.Error("DBFor(read) should return replica")
	}
}

func TestClientSetPoolConfig(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, "postgres://localhost:5432/test?sslmode=disable", "")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// Set pool config
	client.SetPoolConfig(50, 10, 10*time.Minute)

	// Verify config was applied (can't directly check, but no panic = success)
}

func TestClientClose(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, "postgres://localhost:5432/test?sslmode=disable", "")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Close should not panic even if already closed
	client.Close()
	client.Close() // Second close should be safe
}

func TestClientDBForEmptyString(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	// Test with empty string replica (same as "")
	ctx := context.Background()
	client, err := NewClient(ctx, "postgres://localhost:5432/test?sslmode=disable", "")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// DBFor("") should return writeDB
	if client.DBFor("") == nil {
		t.Error("DBFor(empty) should return writeDB")
	}
}

func TestClientDBForUnknownType(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("skipping dbintegration test in CI")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, "postgres://localhost:5432/test?sslmode=disable", "")
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}
	defer client.Close()

	// DBFor("unknown") should return writeDB (fallback)
	if client.DBFor("unknown") == nil {
		t.Error("DBFor(unknown) should return writeDB")
	}
}
