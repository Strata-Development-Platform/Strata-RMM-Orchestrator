package remote

import (
	"database/sql"
	"fmt"
	"time"
)

type RecordingStore struct {
	db *sql.DB
}

type Recording struct {
	ID             string     `json:"id"`
	SessionID      string     `json:"session_id"`
	TenantID       string     `json:"tenant_id"`
	DeviceID       string     `json:"device_id"`
	UserID         *string    `json:"user_id,omitempty"`
	StorageKey     string     `json:"storage_key"`
	SizeBytes      int64      `json:"size_bytes"`
	DurationMs     int64      `json:"duration_ms"`
	Format         string     `json:"format"`
	ChecksumSHA256 string     `json:"checksum_sha256"`
	StorageBackend string     `json:"storage_backend"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func NewRecordingStore(db *sql.DB) *RecordingStore {
	return &RecordingStore{db: db}
}

func (s *RecordingStore) Create(rec *Recording) error {
	_, err := s.db.Exec(`
		INSERT INTO session_recordings (
			id, session_id, tenant_id, device_id, user_id,
			storage_key, size_bytes, duration_ms, format,
			checksum_sha256, storage_backend, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`, rec.ID, rec.SessionID, rec.TenantID, rec.DeviceID, rec.UserID,
		rec.StorageKey, rec.SizeBytes, rec.DurationMs, rec.Format,
		rec.ChecksumSHA256, rec.StorageBackend, rec.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create recording: %w", err)
	}
	return nil
}

func (s *RecordingStore) GetByID(id string) (*Recording, error) {
	var rec Recording
	var userID sql.NullString
	var expiresAt sql.NullTime

	err := s.db.QueryRow(`
		SELECT id, session_id, tenant_id, device_id, user_id,
			storage_key, size_bytes, duration_ms, format,
			checksum_sha256, storage_backend, expires_at, created_at
		FROM session_recordings WHERE id = $1
	`, id).Scan(
		&rec.ID, &rec.SessionID, &rec.TenantID, &rec.DeviceID, &userID,
		&rec.StorageKey, &rec.SizeBytes, &rec.DurationMs, &rec.Format,
		&rec.ChecksumSHA256, &rec.StorageBackend, &expiresAt, &rec.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("recording not found: %s", id)
		}
		return nil, fmt.Errorf("get recording: %w", err)
	}

	if userID.Valid {
		rec.UserID = &userID.String
	}
	if expiresAt.Valid {
		rec.ExpiresAt = &expiresAt.Time
	}
	return &rec, nil
}

func (s *RecordingStore) ListByTenant(tenantID string, limit, offset int) ([]*Recording, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, session_id, tenant_id, device_id, user_id,
			storage_key, size_bytes, duration_ms, format,
			checksum_sha256, storage_backend, expires_at, created_at
		FROM session_recordings
		WHERE tenant_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3
	`, tenantID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list recordings: %w", err)
	}
	defer rows.Close()

	var result []*Recording
	for rows.Next() {
		var rec Recording
		var userID sql.NullString
		var expiresAt sql.NullTime

		if err := rows.Scan(
			&rec.ID, &rec.SessionID, &rec.TenantID, &rec.DeviceID, &userID,
			&rec.StorageKey, &rec.SizeBytes, &rec.DurationMs, &rec.Format,
			&rec.ChecksumSHA256, &rec.StorageBackend, &expiresAt, &rec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan recording: %w", err)
		}
		if userID.Valid {
			rec.UserID = &userID.String
		}
		if expiresAt.Valid {
			rec.ExpiresAt = &expiresAt.Time
		}
		result = append(result, &rec)
	}
	return result, rows.Err()
}

func (s *RecordingStore) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM session_recordings WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete recording: %w", err)
	}
	return nil
}

func (s *RecordingStore) DeleteExpired() ([]string, error) {
	rows, err := s.db.Query(`
		DELETE FROM session_recordings
		WHERE expires_at IS NOT NULL AND expires_at < NOW()
		RETURNING id
	`)
	if err != nil {
		return nil, fmt.Errorf("delete expired: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
