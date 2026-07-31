package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// PostgresQuiescer publishes the recovery mutation gate shared by all
// orchestrator replicas.
type PostgresQuiescer struct {
	db          *sql.DB
	operationID string
}

func NewPostgresQuiescer(db *sql.DB, operationID string) (*PostgresQuiescer, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL connection is required for quiescing")
	}
	if operationID == "" {
		return nil, errors.New("recovery operation identity is required for quiescing")
	}
	return &PostgresQuiescer{db: db, operationID: operationID}, nil
}

func (q *PostgresQuiescer) Quiesce(ctx context.Context) error {
	result, err := q.db.ExecContext(ctx, `
		UPDATE recovery_mutation_gate
		SET quiesced = TRUE, operation_id = $1, updated_at = NOW()
		WHERE singleton = TRUE AND (quiesced = FALSE OR operation_id = $1)
	`, q.operationID)
	if err != nil {
		return fmt.Errorf("close recovery mutation gate: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect recovery mutation gate: %w", err)
	}
	if affected != 1 {
		return errors.New("recovery mutation gate is owned by another operation")
	}
	return nil
}

func (q *PostgresQuiescer) Resume(ctx context.Context) error {
	result, err := q.db.ExecContext(ctx, `
		UPDATE recovery_mutation_gate
		SET quiesced = FALSE, operation_id = NULL, updated_at = NOW()
		WHERE singleton = TRUE AND operation_id = $1
	`, q.operationID)
	if err != nil {
		return fmt.Errorf("open recovery mutation gate: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect recovery mutation gate release: %w", err)
	}
	if affected != 1 {
		return errors.New("recovery mutation gate was not owned by this operation")
	}
	return nil
}

func (q *PostgresQuiescer) Status(ctx context.Context) (QuiesceStatus, error) {
	var quiesced bool
	var owner sql.NullString
	if err := q.db.QueryRowContext(ctx, `
		SELECT quiesced, operation_id
		FROM recovery_mutation_gate
		WHERE singleton = TRUE
	`).Scan(&quiesced, &owner); err != nil {
		return QuiesceStatus{}, fmt.Errorf("read recovery mutation gate: %w", err)
	}
	components := []string{}
	if owner.Valid {
		components = append(components, "operation:"+owner.String)
	}
	return QuiesceStatus{Quiesced: quiesced, Components: components}, nil
}
