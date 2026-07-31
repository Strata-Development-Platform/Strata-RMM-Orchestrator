package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/postgres"
)

// withRecoveryGate prevents new API mutations while a backup or restore owns
// the environment recovery lock. Existing requests hold a shared advisory lock
// until their handler returns, so recovery waits for them to drain.
func (s *APIServer) withRecoveryGate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isMutationMethod(r.Method) || s.db == nil || s.db.DB() == nil {
			next.ServeHTTP(w, r)
			return
		}
		conn, err := s.db.DB().Conn(r.Context())
		if err != nil {
			writeRecoveryUnavailable(w)
			return
		}
		defer conn.Close()
		acquired, err := trySharedRecoveryLock(r.Context(), conn)
		if err != nil || !acquired {
			writeRecoveryUnavailable(w)
			return
		}
		defer func() {
			var unlocked bool
			_ = conn.QueryRowContext(context.WithoutCancel(r.Context()),
				`SELECT pg_advisory_unlock_shared($1)`, postgres.GetRecoveryLockID()).Scan(&unlocked)
		}()
		var quiesced bool
		if err := conn.QueryRowContext(r.Context(), `
			SELECT quiesced FROM recovery_mutation_gate WHERE singleton = TRUE
		`).Scan(&quiesced); err != nil || quiesced {
			writeRecoveryUnavailable(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func trySharedRecoveryLock(ctx context.Context, conn *sql.Conn) (bool, error) {
	var acquired bool
	err := conn.QueryRowContext(ctx,
		`SELECT pg_try_advisory_lock_shared($1)`, postgres.GetRecoveryLockID()).Scan(&acquired)
	return acquired, err
}

func isMutationMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func writeRecoveryUnavailable(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Retry-After", "5")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": "service mutations are temporarily unavailable during recovery operations",
	})
}
