package platform

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Client support request types

type clientSupportRequest struct {
	ID              string          `json:"id"`
	DeviceID        string          `json:"device_id,omitempty"`
	TenantID        string          `json:"tenant_id"`
	Category        string          `json:"category"`
	Subject         string          `json:"subject"`
	Description     string          `json:"description"`
	Priority        string          `json:"priority"`
	Status          string          `json:"status"`
	PlatformReply   string          `json:"platform_reply,omitempty"`
	PlatformReplyAt *time.Time      `json:"platform_reply_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
	ReplyFrom       string          `json:"reply_from,omitempty"`
	ReplyAt         *time.Time      `json:"reply_at,omitempty"`
	ReplyBody       json.RawMessage `json:"reply_body,omitempty"`
}

type createClientSupportRequest struct {
	DeviceID    string `json:"device_id,omitempty"`
	Category    string `json:"category" validate:"required,oneof=technical billing access software network other"`
	Subject     string `json:"subject" validate:"required"`
	Description string `json:"description" validate:"required"`
	Priority    string `json:"priority"`
}

type replyToClientSupportRequest struct {
	Body      string          `json:"body"`
	IsPublic  bool            `json:"is_public"`
	ReplyBody json.RawMessage `json:"reply_body,omitempty"`
}

type listClientSupportRequests struct {
	Requests []clientSupportRequest `json:"requests"`
	Count    int                    `json:"count"`
}

// Client support request handlers

func (s *APIServer) handleCreateClientSupportRequest(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 102400)
	var req createClientSupportRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.Subject == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "subject is required"})
		return
	}

	if req.Description == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "description is required"})
		return
	}

	if req.Priority == "" {
		req.Priority = "normal"
	}

	if req.Category == "" {
		req.Category = "technical"
	}

	requestID := uuid.New().String()
	createdAt := time.Now().UTC()

	_, err := s.requestDB(r).ExecContext(r.Context(), `
		INSERT INTO client_support_requests (
			id, tenant_id, device_id, category, subject, description, priority, status, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'open', $8, $9)
	`, requestID, clientID, req.DeviceID, req.Category, req.Subject, req.Description, req.Priority, createdAt, createdAt)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"id":         requestID,
		"tenant_id":  clientID,
		"device_id":  req.DeviceID,
		"category":   req.Category,
		"subject":    req.Subject,
		"priority":   req.Priority,
		"status":     "open",
		"created_at": createdAt.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleListClientSupportRequests(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	rows, err := s.requestDB(r).QueryContext(r.Context(), `
		SELECT id, tenant_id, device_id, category, subject, description, priority, 
		       status, created_at, updated_at, platform_reply, platform_reply_at,
		       reply_from, reply_at, reply_body
		FROM client_support_requests
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT 100
	`, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}
	defer func() { _ = rows.Close() }()

	var requests []clientSupportRequest
	for rows.Next() {
		var req clientSupportRequest
		var deviceID, category, subject, description, priority, status sql.NullString
		var createdAt, updatedAt time.Time
		var platformReply sql.NullString
		var platformReplyAt, replyAt sql.NullTime
		var replyFrom sql.NullString
		var replyBodyBytes []byte

		if err := rows.Scan(
			&req.ID, &req.TenantID, &deviceID, &category, &subject,
			&description, &priority, &status, &createdAt, &updatedAt,
			&platformReply, &platformReplyAt, &replyFrom, &replyAt, &replyBodyBytes,
		); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
			return
		}

		req.DeviceID = deviceID.String
		req.Category = category.String
		req.Subject = subject.String
		req.Description = description.String
		req.Priority = priority.String
		req.Status = status.String
		req.CreatedAt = createdAt
		req.UpdatedAt = updatedAt
		req.PlatformReply = platformReply.String
		req.PlatformReplyAt = nullableTimeToPtr(platformReplyAt.Time, platformReplyAt.Valid)
		req.ReplyFrom = replyFrom.String
		req.ReplyAt = nullableTimeToPtr(replyAt.Time, replyAt.Valid)
		if len(replyBodyBytes) > 0 {
			req.ReplyBody = json.RawMessage(replyBodyBytes)
		}

		requests = append(requests, req)
	}

	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	if requests == nil {
		requests = []clientSupportRequest{}
	}

	writeJSON(w, http.StatusOK, listClientSupportRequests{Requests: requests, Count: len(requests)})
}

func (s *APIServer) handleGetClientSupportRequest(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	requestID := r.PathValue("requestID")

	if !s.AuthorizeClientAccess(w, r, clientID) {
		return
	}

	var req clientSupportRequest
	var deviceID, category, subject, description, priority, status sql.NullString
	var createdAt, updatedAt time.Time
	var platformReply sql.NullString
	var platformReplyAt, replyAt sql.NullTime
	var replyFrom sql.NullString
	var replyBodyBytes []byte

	err := s.requestDB(r).QueryRowContext(r.Context(), `
		SELECT id, tenant_id, device_id, category, subject, description, priority, 
		       status, created_at, updated_at, platform_reply, platform_reply_at,
		       reply_from, reply_at, reply_body
		FROM client_support_requests
		WHERE id = $1 AND tenant_id = $2
	`, requestID, clientID).Scan(
		&req.ID, &req.TenantID, &deviceID, &category, &subject,
		&description, &priority, &status, &createdAt, &updatedAt,
		&platformReply, &platformReplyAt, &replyFrom, &replyAt, &replyBodyBytes,
	)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "support request not found"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	req.DeviceID = deviceID.String
	req.Category = category.String
	req.Subject = subject.String
	req.Description = description.String
	req.Priority = priority.String
	req.Status = status.String
	req.CreatedAt = createdAt
	req.UpdatedAt = updatedAt
	req.PlatformReply = platformReply.String
	req.PlatformReplyAt = nullableTimeToPtr(platformReplyAt.Time, platformReplyAt.Valid)
	req.ReplyFrom = replyFrom.String
	req.ReplyAt = nullableTimeToPtr(replyAt.Time, replyAt.Valid)
	if len(replyBodyBytes) > 0 {
		req.ReplyBody = json.RawMessage(replyBodyBytes)
	}

	writeJSON(w, http.StatusOK, req)
}

func (s *APIServer) handleReplyToClientSupportRequest(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	requestID := r.PathValue("requestID")

	if !s.AuthorizeClientManage(w, r, clientID) {
		return
	}

	approverID, _ := r.Context().Value(ctxKeyUserID).(string)

	r.Body = http.MaxBytesReader(w, r.Body, 102400)
	var req replyToClientSupportRequest
	if err := decodeStrictJSON(r, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}

	if req.Body == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "reply body is required"})
		return
	}

	now := time.Now().UTC()
	var replyBody []byte
	if len(req.ReplyBody) > 0 {
		replyBody = req.ReplyBody
	} else {
		replyBody, _ = json.Marshal(map[string]interface{}{"body": req.Body, "is_public": req.IsPublic})
	}

	_, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE client_support_requests
		SET platform_reply = $3, platform_reply_at = $4, reply_from = $5, reply_at = $6, reply_body = $7,
		    updated_at = $8, status = 'in_progress'
		WHERE id = $1 AND tenant_id = $2 AND status = 'open'
	`, requestID, clientID, req.Body, now, approverID, now, replyBody, now)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":            requestID,
		"status":        "in_progress",
		"reply_sent_at": now.UTC().Format(time.RFC3339),
	})
}

func (s *APIServer) handleCloseClientSupportRequest(w http.ResponseWriter, r *http.Request) {
	clientID := r.PathValue("clientID")
	requestID := r.PathValue("requestID")

	if !s.AuthorizeClientManage(w, r, clientID) {
		return
	}

	result, err := s.requestDB(r).ExecContext(r.Context(), `
		UPDATE client_support_requests
		SET status = 'closed', updated_at = NOW()
		WHERE id = $1 AND tenant_id = $2 AND status IN ('open', 'in_progress')
	`, requestID, clientID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": sanitizeDBError(err.Error())})
		return
	}

	affected, _ := result.RowsAffected()
	if affected != 1 {
		writeAuthorizationDenied(w)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"id":     requestID,
		"status": "closed",
	})
}

func nullableTimeToPtr(t time.Time, valid bool) *time.Time {
	if !valid {
		return nil
	}
	return &t
}
