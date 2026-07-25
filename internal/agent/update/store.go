package update

import (
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

type Store struct {
	db *bbolt.DB
}

func NewStore(db *bbolt.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Init() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("update"))
		return err
	})
}

func (s *Store) GetState() (*UpdateState, error) {
	var state UpdateState
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("update"))
		if b == nil {
			return fmt.Errorf("update bucket not found")
		}
		data := b.Get([]byte("state"))
		if data == nil {
			state = UpdateState{
				Status:    StatusUpToDate,
				LastCheck: time.Now(),
			}
			return nil
		}
		return json.Unmarshal(data, &state)
	})
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (s *Store) SetState(state *UpdateState) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("update"))
		if b == nil {
			return fmt.Errorf("update bucket not found")
		}
		state.UpdatedAt = time.Now()
		data, err := json.Marshal(state)
		if err != nil {
			return err
		}
		return b.Put([]byte("state"), data)
	})
}

func (s *Store) SetCurrentVersion(version string) error {
	state, err := s.GetState()
	if err != nil {
		return err
	}
	state.CurrentVersion = version
	state.Status = StatusUpToDate
	state.PendingVersion = ""
	state.FailedAttempts = 0
	return s.SetState(state)
}

func (s *Store) SetPendingVersion(version string) error {
	state, err := s.GetState()
	if err != nil {
		return err
	}
	state.RollbackVersion = state.CurrentVersion
	state.PendingVersion = version
	state.Status = StatusPending
	return s.SetState(state)
}

func (s *Store) RecordFailure() error {
	state, err := s.GetState()
	if err != nil {
		return err
	}
	state.FailedAttempts++
	if state.FailedAttempts >= 3 {
		state.Status = StatusFailed
	}
	return s.SetState(state)
}

func (s *Store) SetLastCheck(t time.Time) error {
	state, err := s.GetState()
	if err != nil {
		return err
	}
	state.LastCheck = t
	return s.SetState(state)
}
