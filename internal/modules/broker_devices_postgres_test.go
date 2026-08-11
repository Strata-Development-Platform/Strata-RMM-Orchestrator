package modules

import (
	"errors"
	"testing"
)

func TestValidateResolvedBrokerDeviceScope(t *testing.T) {
	tests := []struct {
		name        string
		msp         string
		client      string
		site        string
		clientMSP   string
		siteClient  string
		want        ResourceScope
		wantErr     bool
	}{
		{
			name: "msp scoped",
			msp:  "msp-1",
			want: ResourceScope{MSPID: "msp-1"},
		},
		{
			name:       "client scoped",
			msp:        "msp-1",
			client:     "client-1",
			clientMSP:  "msp-1",
			want:       ResourceScope{MSPID: "msp-1", ClientID: "client-1"},
		},
		{
			name:       "site scoped",
			msp:        "msp-1",
			client:     "client-1",
			site:       "site-1",
			clientMSP:  "msp-1",
			siteClient: "client-1",
			want:       ResourceScope{MSPID: "msp-1", ClientID: "client-1", SiteID: "site-1"},
		},
		{
			name:      "cross msp client denied",
			msp:       "msp-1",
			client:    "client-1",
			clientMSP: "msp-2",
			wantErr:   true,
		},
		{
			name:       "sibling client site denied",
			msp:        "msp-1",
			client:     "client-1",
			site:       "site-1",
			clientMSP:  "msp-1",
			siteClient: "client-2",
			wantErr:    true,
		},
		{
			name:       "client without msp denied",
			client:     "client-1",
			clientMSP:  "msp-1",
			wantErr:    true,
		},
		{
			name:       "site without client denied",
			msp:        "msp-1",
			site:       "site-1",
			siteClient: "client-1",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateResolvedBrokerDeviceScope(tt.msp, tt.client, tt.site, tt.clientMSP, tt.siteClient)
			if tt.wantErr {
				if !errors.Is(err, ErrBrokerScopeInvalid) {
					t.Fatalf("error = %v, want ErrBrokerScopeInvalid", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("validate scope: %v", err)
			}
			if got != tt.want {
				t.Fatalf("scope = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNewPostgresBrokerDeviceResolverRequiresDatabase(t *testing.T) {
	if _, err := NewPostgresBrokerDeviceResolver(nil); err == nil {
		t.Fatal("expected nil database rejection")
	}
}
