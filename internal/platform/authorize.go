package platform

import (
	"fmt"
	"net/http"
)

// AuthorizeMSPAccess checks whether the authenticated principal can access the given MSP.
// If not, it writes an error response and returns false.
func (s *APIServer) AuthorizeMSPAccess(w http.ResponseWriter, r *http.Request, mspID string) bool {
	principal, ok := r.Context().Value(ctxKeyMSPID).(string)
	if !ok || principal == "" {
		http.Error(w, `{"error":"no msp context"}`, http.StatusForbidden)
		return false
	}
	// Platform admin can access any MSP
	roles, _ := r.Context().Value(ctxKeyRole).(string)
	if roles == "admin" {
		return true
	}
	if principal != mspID {
		http.Error(w, fmt.Sprintf(`{"error":"access to MSP %s denied"}`, mspID), http.StatusForbidden)
		return false
	}
	return true
}

// AuthorizeClientAccess checks whether the authenticated principal can access the given client.
func (s *APIServer) AuthorizeClientAccess(w http.ResponseWriter, r *http.Request, clientID string) bool {
	client, ok := r.Context().Value(ctxKeyClientID).(string)
	if !ok || client == "" {
		// Allow if role is admin (admins can access all)
		roles, _ := r.Context().Value(ctxKeyRole).(string)
		if roles == "admin" {
			return true
		}
		http.Error(w, `{"error":"no client context"}`, http.StatusForbidden)
		return false
	}
	roles, _ := r.Context().Value(ctxKeyRole).(string)
	if roles == "admin" {
		return true
	}
	if client != clientID {
		http.Error(w, fmt.Sprintf(`{"error":"access to client %s denied"}`, clientID), http.StatusForbidden)
		return false
	}
	return true
}

// AuthorizeSiteAccess checks whether the authenticated principal can access the given site.
func (s *APIServer) AuthorizeSiteAccess(w http.ResponseWriter, r *http.Request, siteID string) bool {
	site, ok := r.Context().Value(ctxKeySiteID).(string)
	if !ok || site == "" {
		roles, _ := r.Context().Value(ctxKeyRole).(string)
		if roles == "admin" {
			return true
		}
		return true // If no site scope in token, allow (for now)
	}
	if site != siteID {
		http.Error(w, fmt.Sprintf(`{"error":"access to site %s denied"}`, siteID), http.StatusForbidden)
		return false
	}
	return true
}
