package platform

import (
	"fmt"
	"net/http"
	"strings"
)

func getRoles(r *http.Request) []string {
	rolesStr, _ := r.Context().Value(ctxKeyRole).(string)
	if rolesStr == "" {
		return nil
	}
	return strings.Split(rolesStr, ",")
}

func hasAdminRole(roles []string) bool {
	for _, r := range roles {
		switch r {
		case "admin", "platform_owner", "platform_admin", "msp_owner", "msp_admin":
			return true
		}
	}
	return false
}

// AuthorizeMSPAccess checks whether the authenticated principal can access the given MSP.
func (s *APIServer) AuthorizeMSPAccess(w http.ResponseWriter, r *http.Request, mspID string) bool {
	principalMSP, _ := r.Context().Value(ctxKeyMSPID).(string)
	roles := getRoles(r)

	if hasAdminRole(roles) {
		return true
	}
	if principalMSP == "" {
		http.Error(w, `{"error":"no msp context"}`, http.StatusForbidden)
		return false
	}
	if principalMSP != mspID {
		http.Error(w, fmt.Sprintf(`{"error":"access to MSP %s denied"}`, mspID), http.StatusForbidden)
		return false
	}
	return true
}

// AuthorizeClientAccess checks whether the authenticated principal can access the given client.
func (s *APIServer) AuthorizeClientAccess(w http.ResponseWriter, r *http.Request, clientID string) bool {
	principalClient, _ := r.Context().Value(ctxKeyClientID).(string)
	roles := getRoles(r)

	if hasAdminRole(roles) {
		return true
	}
	if principalClient == "" {
		http.Error(w, `{"error":"no client context"}`, http.StatusForbidden)
		return false
	}
	if principalClient != clientID {
		http.Error(w, fmt.Sprintf(`{"error":"access to client %s denied"}`, clientID), http.StatusForbidden)
		return false
	}
	return true
}

// AuthorizeSiteAccess checks whether the authenticated principal can access the given site.
func (s *APIServer) AuthorizeSiteAccess(w http.ResponseWriter, r *http.Request, siteID string) bool {
	principalSite, _ := r.Context().Value(ctxKeySiteID).(string)
	roles := getRoles(r)

	if hasAdminRole(roles) {
		return true
	}
	if principalSite == "" {
		http.Error(w, `{"error":"no site context"}`, http.StatusForbidden)
		return false
	}
	if principalSite != siteID {
		http.Error(w, fmt.Sprintf(`{"error":"access to site %s denied"}`, siteID), http.StatusForbidden)
		return false
	}
	return true
}
