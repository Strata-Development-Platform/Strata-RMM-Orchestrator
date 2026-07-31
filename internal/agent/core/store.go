package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

type Store struct {
	db       *bbolt.DB
	maxItems int
}

var ErrQueueFull = errors.New("offline queue capacity exhausted")

type StoredMetric struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Tags      map[string]string `json:"tags"`
	Timestamp time.Time         `json:"timestamp"`
}

type StoredEvent struct {
	Type      string            `json:"type"`
	Message   string            `json:"message"`
	Tags      map[string]string `json:"tags"`
	Timestamp time.Time         `json:"timestamp"`
}

type QueuedMetric struct {
	Key    string
	Metric StoredMetric
}

type QueuedEvent struct {
	Key   string
	Event StoredEvent
}

func NewStore(path string) (*Store, error) {
	return NewStoreWithLimit(path, 10000)
}

func NewStoreWithLimit(path string, maxItems int) (*Store, error) {
	if maxItems <= 0 {
		return nil, fmt.Errorf("queue limit must be greater than zero")
	}
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}

	s := &Store{db: db, maxItems: maxItems}
	if err := s.init(); err != nil {
		db.Close()
		return nil, err
	}

	return s, nil
}

func (s *Store) init() error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		buckets := []string{"metrics", "events", "state", "queue"}
		for _, name := range buckets {
			if _, err := tx.CreateBucketIfNotExists([]byte(name)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) QueueMetric(m StoredMetric) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("metrics"))
		if s.queueSize(tx) >= s.maxItems {
			return ErrQueueFull
		}
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		sequence, err := b.NextSequence()
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%020d:%s:%d", sequence, m.Name, m.Timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
}

func (s *Store) QueueEvent(e StoredEvent) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("events"))
		if s.queueSize(tx) >= s.maxItems {
			return ErrQueueFull
		}
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		sequence, err := b.NextSequence()
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%020d:%s:%d", sequence, e.Type, e.Timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
}

func (s *Store) queueSize(tx *bbolt.Tx) int {
	return tx.Bucket([]byte("metrics")).Stats().KeyN + tx.Bucket([]byte("events")).Stats().KeyN
}

func (s *Store) PopMetrics(limit int) ([]StoredMetric, error) {
	var result []StoredMetric
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("metrics"))
		c := b.Cursor()
		count := 0
		for k, v := c.First(); k != nil && count < limit; k, v = c.Next() {
			var m StoredMetric
			if err := json.Unmarshal(v, &m); err != nil {
				continue
			}
			result = append(result, m)
			if err := b.Delete(k); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return result, err
}

func (s *Store) PopEvents(limit int) ([]StoredEvent, error) {
	var result []StoredEvent
	err := s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("events"))
		c := b.Cursor()
		count := 0
		for k, v := c.First(); k != nil && count < limit; k, v = c.Next() {
			var e StoredEvent
			if err := json.Unmarshal(v, &e); err != nil {
				continue
			}
			result = append(result, e)
			if err := b.Delete(k); err != nil {
				return err
			}
			count++
		}
		return nil
	})
	return result, err
}

// PeekMetrics reads queued metrics without deleting them. Call AckMetrics only
// after the corresponding publish has completed successfully.
func (s *Store) PeekMetrics(limit int) ([]QueuedMetric, error) {
	var result []QueuedMetric
	err := s.db.View(func(tx *bbolt.Tx) error {
		cursor := tx.Bucket([]byte("metrics")).Cursor()
		for key, value := cursor.First(); key != nil && len(result) < limit; key, value = cursor.Next() {
			var metric StoredMetric
			if err := json.Unmarshal(value, &metric); err != nil {
				return fmt.Errorf("decode queued metric %q: %w", key, err)
			}
			result = append(result, QueuedMetric{Key: string(key), Metric: metric})
		}
		return nil
	})
	return result, err
}

func (s *Store) AckMetrics(keys []string) error {
	return s.ack("metrics", keys)
}

func (s *Store) PeekEvents(limit int) ([]QueuedEvent, error) {
	var result []QueuedEvent
	err := s.db.View(func(tx *bbolt.Tx) error {
		cursor := tx.Bucket([]byte("events")).Cursor()
		for key, value := cursor.First(); key != nil && len(result) < limit; key, value = cursor.Next() {
			var event StoredEvent
			if err := json.Unmarshal(value, &event); err != nil {
				return fmt.Errorf("decode queued event %q: %w", key, err)
			}
			result = append(result, QueuedEvent{Key: string(key), Event: event})
		}
		return nil
	})
	return result, err
}

func (s *Store) AckEvents(keys []string) error {
	return s.ack("events", keys)
}

func (s *Store) ack(bucket string, keys []string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		for _, key := range keys {
			if err := b.Delete([]byte(key)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) PutState(key string, value []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("state"))
		return b.Put([]byte(key), value)
	})
}

func (s *Store) GetState(key string) ([]byte, error) {
	var value []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("state"))
		value = b.Get([]byte(key))
		return nil
	})
	return value, err
}

func (s *Store) QueueSize() (int, error) {
	var count int
	err := s.db.View(func(tx *bbolt.Tx) error {
		count = s.queueSize(tx)
		return nil
	})
	return count, err
}

func (s *Store) DB() *bbolt.DB {
	return s.db
}

func (s *Store) Close() error {
	return s.db.Close()
}
