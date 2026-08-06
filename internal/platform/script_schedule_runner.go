package platform

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ScriptScheduleRunner periodically evaluates active schedules and dispatches
// script execution to matching devices when a schedule becomes due.
type ScriptScheduleRunner struct {
	interval time.Duration
	so       *ScheduleOrchestrator
	logger   *zap.Logger
	stopCh   chan struct{}
	mu       sync.Mutex
	running  bool
}

// NewScriptScheduleRunner creates a new schedule runner engine.
func NewScriptScheduleRunner(interval time.Duration, so *ScheduleOrchestrator, logger *zap.Logger) *ScriptScheduleRunner {
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	return &ScriptScheduleRunner{
		interval: interval,
		so:       so,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the periodic schedule evaluation loop.
func (r *ScriptScheduleRunner) Start(ctx context.Context) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	go r.runLoop(ctx)
}

// Stop signals the schedule runner to stop.
func (r *ScriptScheduleRunner) Stop() {
	close(r.stopCh)
	r.mu.Lock()
	r.running = false
	r.mu.Unlock()
}

// Healthy returns nil if the runner is running, or an error if stopped.
func (r *ScriptScheduleRunner) Healthy(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.running {
		return context.DeadlineExceeded
	}
	return nil
}

func (r *ScriptScheduleRunner) runLoop(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	r.logger.Info("script schedule runner started", zap.Duration("interval", r.interval))

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("script schedule runner stopped: context done")
			return
		case <-r.stopCh:
			r.logger.Info("script schedule runner stopped")
			return
		case <-ticker.C:
			r.evaluateSchedules(ctx)
		}
	}
}

// evaluateSchedules finds all active schedules with next_run_at in the past
// and dispatches them for execution.
func (r *ScriptScheduleRunner) evaluateSchedules(ctx context.Context) {
	start := time.Now()

	scheduleIDs, err := r.getDueSchedules(ctx)
	if err != nil {
		r.logger.Error("getting due schedules", zap.Error(err))
		return
	}

	if len(scheduleIDs) == 0 {
		return
	}

	r.logger.Info("evaluating due schedules", zap.Int("count", len(scheduleIDs)))

	for _, scheduleID := range scheduleIDs {
		select {
		case <-ctx.Done():
			return
		default:
			bindingIDs, err := r.getSmartGroupBindingsForSchedule(ctx, scheduleID)
			if err != nil {
				r.logger.Error("querying smart group bindings",
					zap.String("schedule_id", scheduleID),
					zap.Error(err))
			}

			if len(bindingIDs) > 0 {
				for _, bindingID := range bindingIDs {
					if err := r.dispatchScheduleToSmartGroup(ctx, scheduleID, bindingID); err != nil {
						r.logger.Error("dispatching schedule to smart group",
							zap.String("schedule_id", scheduleID),
							zap.String("binding_id", bindingID),
							zap.Error(err))
					}
				}
			} else {
				if err := r.so.ExecuteSchedule(scheduleID); err != nil {
					r.logger.Error("executing schedule",
						zap.String("schedule_id", scheduleID),
						zap.Error(err))
				}
			}
		}
	}

	elapsed := time.Since(start)
	r.logger.Info("schedule evaluation cycle complete",
		zap.Int("schedule_count", len(scheduleIDs)),
		zap.Duration("duration", elapsed))
}

// getDueSchedules returns IDs of active schedules where next_run_at is in the
// past. This query is intentionally lightweight to run every evaluation tick.
func (r *ScriptScheduleRunner) getDueSchedules(ctx context.Context) ([]string, error) {
	rows, err := r.so.db.QueryContext(ctx, `
		SELECT id FROM schedules
		WHERE status = 'active'
		  AND next_run_at <= NOW()
		ORDER BY next_run_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func (r *ScriptScheduleRunner) getSmartGroupBindingsForSchedule(ctx context.Context, scheduleID string) ([]string, error) {
	rows, err := r.so.db.QueryContext(ctx, `
		SELECT id::text FROM smart_group_script_bindings
		WHERE schedule_id = $1 AND enabled = true
		ORDER BY priority ASC
	`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var bindingIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		bindingIDs = append(bindingIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return bindingIDs, nil
}

type ScheduleOrchestratorDispatch struct {
	TargetDevice string
	TargetGroup  string
	ScriptID     string
	Payload      json.RawMessage
}

func (r *ScriptScheduleRunner) dispatchScheduleToSmartGroup(ctx context.Context, scheduleID, bindingID string) error {
	var groupID string
	var bindingType string
	var mspID string
	err := r.so.db.QueryRowContext(ctx, `
		SELECT group_id::text, binding_type, msp_id::text
		FROM smart_group_script_bindings
		WHERE id = $1
	`, bindingID).Scan(&groupID, &bindingType, &mspID)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	deviceIDs, err := r.getSmartGroupMembers(ctx, groupID)
	if err != nil {
		return err
	}

	if len(deviceIDs) == 0 {
		r.logger.Info("smart group has no members for schedule dispatch",
			zap.String("schedule_id", scheduleID),
			zap.String("group_id", groupID))
		return nil
	}

	scheduleInfo, err := r.getScheduleInfo(ctx, scheduleID)
	if err != nil {
		return err
	}

	for _, deviceID := range deviceIDs {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			if err := r.dispatchToDevice(ctx, scheduleInfo, deviceID, bindingID); err != nil {
				r.logger.Error("dispatching to device",
					zap.String("device_id", deviceID),
					zap.String("schedule_id", scheduleID),
					zap.String("binding_id", bindingID),
					zap.Error(err))
			}
		}
	}

	r.logger.Info("smart group schedule dispatch complete",
		zap.String("schedule_id", scheduleID),
		zap.String("binding_id", bindingID),
		zap.String("group_id", groupID),
		zap.Int("member_count", len(deviceIDs)))
	return nil
}

func (r *ScriptScheduleRunner) getSmartGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	rows, err := r.so.db.QueryContext(ctx, `
		SELECT device_id::text FROM group_memberships
		WHERE group_id = $1
	`, groupID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var deviceIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		deviceIDs = append(deviceIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return deviceIDs, nil
}

type scheduleInfo struct {
	ID       string
	ScriptID string
}

func (r *ScriptScheduleRunner) getScheduleInfo(ctx context.Context, scheduleID string) (*scheduleInfo, error) {
	var info scheduleInfo
	err := r.so.db.QueryRowContext(ctx, `
		SELECT id::text, script_id::text FROM schedules WHERE id = $1
	`, scheduleID).Scan(&info.ID, &info.ScriptID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("schedule %s not found", scheduleID)
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *ScriptScheduleRunner) dispatchToDevice(ctx context.Context, info *scheduleInfo, deviceID, bindingID string) error {
	return r.so.ExecuteScheduleDevice(ctx, info.ID, deviceID)
}
