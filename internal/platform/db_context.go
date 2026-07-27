package platform

import (
	"bytes"
	"context"
	"database/sql"
	"net/http"
)

type dbExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}

type transactionResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func newTransactionResponse() *transactionResponse {
	return &transactionResponse{header: make(http.Header), status: http.StatusOK}
}

func (w *transactionResponse) Header() http.Header {
	return w.header
}

func (w *transactionResponse) WriteHeader(status int) {
	if w.status != http.StatusOK || status == http.StatusOK {
		return
	}
	w.status = status
}

func (w *transactionResponse) Write(data []byte) (int, error) {
	return w.body.Write(data)
}

func (w *transactionResponse) flushTo(destination http.ResponseWriter) {
	for key, values := range w.header {
		for _, value := range values {
			destination.Header().Add(key, value)
		}
	}
	destination.WriteHeader(w.status)
	_, _ = destination.Write(w.body.Bytes())
}

func (s *APIServer) requestDB(r *http.Request) dbExecutor {
	if tx, ok := r.Context().Value(ctxKeyDBTransaction).(*sql.Tx); ok && tx != nil {
		return tx
	}
	return s.db.DB()
}

func (s *APIServer) withTenantTransaction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, _ := r.Context().Value(ctxKeyUserID).(string)
		tokenUse, _ := r.Context().Value(ctxKeyTokenUse).(string)
		if s.db == nil || userID == "" || tokenUse != "user" {
			next.ServeHTTP(w, r)
			return
		}

		tx, err := s.db.DB().BeginTx(r.Context(), nil)
		if err != nil {
			http.Error(w, "database transaction unavailable", http.StatusServiceUnavailable)
			return
		}
		committed := false
		defer func() {
			if !committed {
				_ = tx.Rollback()
			}
		}()

		mspID, _ := r.Context().Value(ctxKeyMSPID).(string)
		clientID, _ := r.Context().Value(ctxKeyClientID).(string)
		siteID, _ := r.Context().Value(ctxKeySiteID).(string)
		tenantID, _ := r.Context().Value(ctxKeyTenantID).(string)
		supportGrantID, _ := r.Context().Value(ctxKeySupportGrantID).(string)
		role := ""
		if isPlatformGlobal(getRoles(r)) {
			role = "platform_admin"
		}
		permission := "read"
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			permission = "write"
		}

		if _, err := tx.ExecContext(r.Context(), `
			SELECT
				set_config('app.user_id', $1, true),
				set_config('app.msp_id', $2, true),
				set_config('app.client_id', $3, true),
				set_config('app.site_id', $4, true),
				set_config('app.role', $5, true),
				set_config('app.support_grant_id', $6, true),
				set_config('app.permission', $7, true),
				set_config('app.tenant_id', $8, true)
		`, userID, mspID, clientID, siteID, role, supportGrantID, permission, tenantID); err != nil {
			http.Error(w, "database security context unavailable", http.StatusServiceUnavailable)
			return
		}

		buffered := newTransactionResponse()
		ctx := context.WithValue(r.Context(), ctxKeyDBTransaction, tx)
		next.ServeHTTP(buffered, r.WithContext(ctx))
		if buffered.status >= http.StatusBadRequest {
			_ = tx.Rollback()
			buffered.flushTo(w)
			return
		}
		if err := tx.Commit(); err != nil {
			http.Error(w, "database transaction failed", http.StatusInternalServerError)
			return
		}
		committed = true
		buffered.flushTo(w)
	})
}
