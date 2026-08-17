package platform

import "fmt"

type JobState string

const (
	JobPending    JobState = "pending"
	JobQueued     JobState = "queued"
	JobDispatched JobState = "dispatched"
	JobRunning    JobState = "running"
	JobSucceeded  JobState = "succeeded"
	JobFailed     JobState = "failed"
	JobCancelled  JobState = "cancelled"
	JobExpired    JobState = "expired"
)

var validStates = map[JobState]bool{
	JobPending: true, JobQueued: true, JobDispatched: true,
	JobRunning: true, JobSucceeded: true, JobFailed: true,
	JobCancelled: true, JobExpired: true,
}

var allowedTransitions = map[JobState]map[JobState]bool{
	JobPending:    {JobQueued: true, JobCancelled: true, JobExpired: true},
	JobQueued:     {JobDispatched: true, JobCancelled: true, JobExpired: true},
	JobDispatched: {JobRunning: true, JobSucceeded: true, JobFailed: true, JobCancelled: true, JobExpired: true},
	JobRunning:    {JobSucceeded: true, JobFailed: true, JobCancelled: true, JobExpired: true},
}

func IsValidState(s string) bool {
	return validStates[JobState(s)]
}

func CanTransition(from, to JobState) bool {
	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	return allowed[to]
}

func TransitionJob(from, to string) error {
	if !IsValidState(from) {
		return fmt.Errorf("invalid source state: %s", from)
	}
	if !IsValidState(to) {
		return fmt.Errorf("invalid target state: %s", to)
	}
	if !CanTransition(JobState(from), JobState(to)) {
		return fmt.Errorf("transition not allowed: %s → %s", from, to)
	}
	return nil
}
