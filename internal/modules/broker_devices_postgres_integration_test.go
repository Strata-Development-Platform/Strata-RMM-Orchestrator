//go:build moduleintegration

package modules

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	_ "github.com/strata-rmm/strata-rmm-orchestrator/internal/postgresdriver"
)

func TestPostgresDeviceBrokerAuthoritativeScopeIsolation(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Fatal("TEST_POSTGRES_DSN is required")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	const schema = "module_broker_scope_test"
	if _, err := db.ExecContext(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE; CREATE SCHEMA `+schema+`; SET search_path TO `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`) }()

	for _, statement := range []string{
		`CREATE TABLE msp_tenants (id UUID PRIMARY KEY)`,
		`CREATE TABLE client_organizations (id UUID PRIMARY KEY, msp_id UUID NOT NULL)`,
		`CREATE TABLE sites (id UUID PRIMARY KEY, client_id UUID NOT NULL)`,
		`CREATE TABLE devices (id UUID PRIMARY KEY, hostname TEXT NOT NULL, status TEXT NOT NULL, msp_id UUID, client_id UUID, site_id UUID)`,
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	const (
		mspA     = "00000000-0000-0000-0000-000000000101"
		mspB     = "00000000-0000-0000-0000-000000000102"
		clientA  = "00000000-0000-0000-0000-000000000201"
		clientB  = "00000000-0000-0000-0000-000000000202"
		siteA    = "00000000-0000-0000-0000-000000000301"
		siteB    = "00000000-0000-0000-0000-000000000302"
		deviceA  = "00000000-0000-0000-0000-000000000401"
		disabled = "00000000-0000-0000-0000-000000000402"
	)
	for _, statement := range []string{
		fmt.Sprintf(`INSERT INTO msp_tenants(id) VALUES ('%s'),('%s')`, mspA, mspB),
		fmt.Sprintf(`INSERT INTO client_organizations(id,msp_id) VALUES ('%s','%s'),('%s','%s')`, clientA, mspA, clientB, mspA),
		fmt.Sprintf(`INSERT INTO sites(id,client_id) VALUES ('%s','%s'),('%s','%s')`, siteA, clientA, siteB, clientB),
		fmt.Sprintf(`INSERT INTO devices(id,hostname,status,msp_id,client_id,site_id) VALUES ('%s','endpoint-a','online','%s','%s','%s'),('%s','endpoint-disabled','disabled','%s','%s','%s')`, deviceA, mspA, clientA, siteA, disabled, mspA, clientA, siteA),
	} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}

	registry, module := enabledBrokerModule(t, []string{"devices.read"})
	runtime, err := NewPostgresWASIRuntime(db, registry, PostgresWASIRuntimeOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if runtime.broker == nil {
		t.Fatal("production WASI runtime has no capability broker")
	}
	resolver, err := NewPostgresBrokerDeviceResolver(db)
	if err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(map[string]string{"device_id": deviceA})
	if err != nil {
		t.Fatal(err)
	}

	exactScope := ResourceScope{MSPID: mspA, ClientID: clientA, SiteID: siteA}
	output, err := runtime.broker.Call(ctx, module, BrokerRequest{Operation: BrokerOperationDevicesGet, Scope: exactScope, Input: input})
	if err != nil {
		t.Fatalf("authorized broker read: %v", err)
	}
	var got brokerDeviceGetResponse
	if err := json.Unmarshal(output, &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != deviceA || got.Hostname != "endpoint-a" || got.Status != "online" {
		t.Fatalf("unexpected broker response: %+v", got)
	}
	if _, err := resolver.ResolveBrokerDevice(ctx, disabled); err == nil {
		t.Fatal("disabled device unexpectedly resolved through broker")
	}

	for name, scope := range map[string]ResourceScope{
		"sibling site":   {MSPID: mspA, ClientID: clientB, SiteID: siteB},
		"sibling client": {MSPID: mspA, ClientID: clientB},
		"cross MSP":      {MSPID: mspB},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := runtime.broker.Call(ctx, module, BrokerRequest{Operation: BrokerOperationDevicesGet, Scope: scope, Input: input})
			if !errors.Is(err, ErrBrokerScopeInvalid) {
				t.Fatalf("error=%v, want ErrBrokerScopeInvalid", err)
			}
		})
	}

	// Corrupt the denormalized device hierarchy so its client belongs to MSP A
	// but the device claims MSP B. The resolver must reject this before the
	// trusted invocation-scope comparison can run.
	if _, err := db.ExecContext(ctx, `UPDATE devices SET msp_id=$1 WHERE id=$2`, mspB, deviceA); err != nil {
		t.Fatal(err)
	}
	if _, err := resolver.ResolveBrokerDevice(ctx, deviceA); !errors.Is(err, ErrBrokerScopeInvalid) {
		t.Fatalf("corrupt hierarchy error=%v, want ErrBrokerScopeInvalid", err)
	}
}
