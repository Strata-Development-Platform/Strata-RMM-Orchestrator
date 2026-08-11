package modules

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// PostgresBrokerDeviceResolver resolves broker-visible device identity and
// organizational ownership from the authoritative control-plane tables. It
// deliberately re-checks client->MSP and site->client ancestry rather than
// trusting the denormalized scope columns on devices in isolation.
type PostgresBrokerDeviceResolver struct {
	db *sql.DB
}

func NewPostgresBrokerDeviceResolver(db *sql.DB) (*PostgresBrokerDeviceResolver, error) {
	if db == nil {
		return nil, errors.New("broker device PostgreSQL database is required")
	}
	return &PostgresBrokerDeviceResolver{db: db}, nil
}

func (r *PostgresBrokerDeviceResolver) ResolveBrokerDevice(ctx context.Context, deviceID string) (BrokerDevice, error) {
	if r == nil || r.db == nil {
		return BrokerDevice{}, errors.New("broker device PostgreSQL resolver is unavailable")
	}
	if ctx == nil {
		return BrokerDevice{}, errors.New("broker device context is required")
	}
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || len(deviceID) > maxBrokerDeviceIDBytes {
		return BrokerDevice{}, errors.New("invalid broker device id")
	}

	var device BrokerDevice
	var deviceMSP, deviceClient, deviceSite string
	var clientMSP, siteClient string
	err := r.db.QueryRowContext(ctx, `
		SELECT d.id::text,
		       d.hostname,
		       d.status,
		       COALESCE(d.msp_id::text, ''),
		       COALESCE(d.client_id::text, ''),
		       COALESCE(d.site_id::text, ''),
		       COALESCE(c.msp_id::text, ''),
		       COALESCE(s.client_id::text, '')
		FROM devices d
		LEFT JOIN client_organizations c ON c.id = d.client_id
		LEFT JOIN sites s ON s.id = d.site_id
		WHERE d.id = $1::uuid
		  AND d.status <> 'disabled'
	`, deviceID).Scan(
		&device.ID,
		&device.Hostname,
		&device.Status,
		&deviceMSP,
		&deviceClient,
		&deviceSite,
		&clientMSP,
		&siteClient,
	)
	if err != nil {
		return BrokerDevice{}, fmt.Errorf("query authoritative broker device: %w", err)
	}

	scope, err := validateResolvedBrokerDeviceScope(deviceMSP, deviceClient, deviceSite, clientMSP, siteClient)
	if err != nil {
		return BrokerDevice{}, err
	}
	device.Scope = scope
	return device, nil
}

func validateResolvedBrokerDeviceScope(deviceMSP, deviceClient, deviceSite, clientMSP, siteClient string) (ResourceScope, error) {
	scope := ResourceScope{
		MSPID:    strings.TrimSpace(deviceMSP),
		ClientID: strings.TrimSpace(deviceClient),
		SiteID:   strings.TrimSpace(deviceSite),
	}
	clientMSP = strings.TrimSpace(clientMSP)
	siteClient = strings.TrimSpace(siteClient)

	if err := validateServiceScope(scope.MSPID, scope.ClientID, scope.SiteID); err != nil {
		return ResourceScope{}, fmt.Errorf("%w: authoritative device scope is invalid", ErrBrokerScopeInvalid)
	}
	if scope.ClientID != "" {
		if clientMSP == "" || clientMSP != scope.MSPID {
			return ResourceScope{}, fmt.Errorf("%w: device client does not belong to device MSP", ErrBrokerScopeInvalid)
		}
	} else if clientMSP != "" {
		return ResourceScope{}, fmt.Errorf("%w: unexpected client ancestry for MSP-scoped device", ErrBrokerScopeInvalid)
	}
	if scope.SiteID != "" {
		if siteClient == "" || siteClient != scope.ClientID {
			return ResourceScope{}, fmt.Errorf("%w: device site does not belong to device client", ErrBrokerScopeInvalid)
		}
	} else if siteClient != "" {
		return ResourceScope{}, fmt.Errorf("%w: unexpected site ancestry for non-site device", ErrBrokerScopeInvalid)
	}
	return scope, nil
}
