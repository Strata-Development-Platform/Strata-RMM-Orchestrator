package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
)

// PostgresOperationLock holds a session-level advisory lock on one pinned
// connection for the complete operation.
type PostgresOperationLock struct {
	db            *sql.DB
	lockID        int64
	retryInterval time.Duration

	mu   sync.Mutex
	conn *sql.Conn
}

func NewPostgresOperationLock(db *sql.DB, lockID int64) (*PostgresOperationLock, error) {
	if db == nil {
		return nil, errors.New("PostgreSQL connection is required for recovery lock")
	}
	if lockID == 0 {
		return nil, errors.New("non-zero recovery advisory lock ID is required")
	}
	return &PostgresOperationLock{db: db, lockID: lockID, retryInterval: 250 * time.Millisecond}, nil
}

func (l *PostgresOperationLock) Acquire(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn != nil {
		return errors.New("recovery advisory lock is already held by this process")
	}
	conn, err := l.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("reserve PostgreSQL lock connection: %w", err)
	}
	for {
		var acquired bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, l.lockID).Scan(&acquired); err != nil {
			_ = conn.Close()
			return fmt.Errorf("acquire PostgreSQL advisory lock: %w", err)
		}
		if acquired {
			l.conn = conn
			return nil
		}
		timer := time.NewTimer(l.retryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = conn.Close()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (l *PostgresOperationLock) Release(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return nil
	}
	conn := l.conn
	l.conn = nil
	var unlocked bool
	err := conn.QueryRowContext(ctx, `SELECT pg_advisory_unlock($1)`, l.lockID).Scan(&unlocked)
	closeErr := conn.Close()
	if err != nil {
		return fmt.Errorf("release PostgreSQL advisory lock: %w", err)
	}
	if !unlocked {
		return errors.New("PostgreSQL advisory lock was not held")
	}
	if closeErr != nil {
		return fmt.Errorf("close PostgreSQL lock connection: %w", closeErr)
	}
	return nil
}
