package platform

import (
	"reflect"
	"testing"
)

func TestPermissionsForRoles(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  []string
	}{
		{
			name:  "platform administrator",
			roles: []string{"platform_admin"},
			want:  []string{"msp:manage", "platform:manage", "security:read", "support:manage"},
		},
		{
			name:  "combined roles are sorted and deduplicated",
			roles: []string{"msp_technician", "msp_viewer"},
			want:  []string{"client:read", "device:manage", "device:read", "job:manage", "job:read", "site:read"},
		},
		{
			name:  "unknown role has no permissions",
			roles: []string{"unknown"},
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := permissionsForRoles(tt.roles)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("permissionsForRoles(%v) = %v, want %v", tt.roles, got, tt.want)
			}
		})
	}
}

func TestContextRouteClassification(t *testing.T) {
	server := &APIServer{}
	if got := server.classifyRoute("GET", "/api/v2/context"); got != AccessUser {
		t.Fatalf("context route access = %v, want AccessUser", got)
	}
	if got := server.classifyRoute("GET", "/api/v2/platform/msps"); got != AccessAdmin {
		t.Fatalf("MSP list route access = %v, want AccessAdmin", got)
	}
	if got := server.classifyRoute("PATCH", "/api/v2/platform/msps/msp-id/entitlement"); got != AccessAdmin {
		t.Fatalf("entitlement update route access = %v, want AccessAdmin", got)
	}
	if got := server.classifyRoute("POST", "/api/v2/platform/msps/msp-id/offboarding"); got != AccessAdmin {
		t.Fatalf("offboarding route access = %v, want AccessAdmin", got)
	}
	if got := server.classifyRoute("POST", "/api/v2/platform/msps/msp-id/offboarding/approve-deletion"); got != AccessAdmin {
		t.Fatalf("deletion approval route access = %v, want AccessAdmin", got)
	}
	if got := server.classifyRoute("GET", "/api/v2/platform/msps/msp-id/export"); got != AccessAdmin {
		t.Fatalf("MSP export route access = %v, want AccessAdmin", got)
	}
	if got := server.classifyRoute("POST", "/api/v2/context/switch"); got != AccessUser {
		t.Fatalf("context switch route access = %v, want AccessUser", got)
	}
}

func TestHostnameValidation(t *testing.T) {
	valid := []string{"rmm.example.com", "portal.msp-example.co.uk"}
	invalid := []string{"example", "https://rmm.example.com", "-bad.example.com", "bad_domain.example.com"}
	for _, hostname := range valid {
		if !hostnamePattern.MatchString(hostname) {
			t.Errorf("expected %q to be valid", hostname)
		}
	}
	for _, hostname := range invalid {
		if hostnamePattern.MatchString(hostname) {
			t.Errorf("expected %q to be invalid", hostname)
		}
	}
}
