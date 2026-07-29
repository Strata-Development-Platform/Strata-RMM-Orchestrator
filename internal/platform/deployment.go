package platform

import (
	"sync"
	"time"
)

type DeploymentState int

const (
	DeploymentStatePending    DeploymentState = iota
	DeploymentStateInProgress
	DeploymentStateCompleted
	DeploymentStateFailed
	DeploymentStateRollingBack
)

func (s DeploymentState) String() string {
	switch s {
	case DeploymentStatePending:
		return "pending"
	case DeploymentStateInProgress:
		return "in_progress"
	case DeploymentStateCompleted:
		return "completed"
	case DeploymentStateFailed:
		return "failed"
	case DeploymentStateRollingBack:
		return "rolling_back"
	default:
		return "unknown"
	}
}

type DeploymentEvent struct {
	ID              string         `json:"id"`
	Version         string         `json:"version"`
	PreviousVersion string         `json:"previous_version"`
	State           DeploymentState `json:"state"`
	Timestamp       time.Time      `json:"timestamp"`
	Error           string         `json:"error,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type DeploymentController struct {
	mu        sync.RWMutex
	events    []DeploymentEvent
	current   DeploymentState
	lastEvent *DeploymentEvent
}

func NewDeploymentController() *DeploymentController {
	return &DeploymentController{
		events:  make([]DeploymentEvent, 0),
		current: DeploymentStatePending,
	}
}

func (c *DeploymentController) RecordEvent(event DeploymentEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()

	event.Timestamp = time.Now()
	c.events = append(c.events, event)
	c.lastEvent = &event
}

func (c *DeploymentController) GetState() DeploymentState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.current
}

func (c *DeploymentController) GetLastEvent() *DeploymentEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastEvent == nil {
		return nil
	}
	event := *c.lastEvent
	return &event
}

func (c *DeploymentController) GetHistory() []DeploymentEvent {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.events) == 0 {
		return nil
	}
	history := make([]DeploymentEvent, len(c.events))
	copy(history, c.events)
	return history
}

func (c *DeploymentController) TransitionTo(newState DeploymentState, errMsg string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.current = newState

	event := DeploymentEvent{
		State:   newState,
		Timestamp: time.Now(),
	}
	if errMsg != "" {
		event.Error = errMsg
	}

	c.events = append(c.events, event)
	c.lastEvent = &event
}

func (c *DeploymentController) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.current = DeploymentStatePending
	c.events = make([]DeploymentEvent, 0)
	c.lastEvent = nil
}
