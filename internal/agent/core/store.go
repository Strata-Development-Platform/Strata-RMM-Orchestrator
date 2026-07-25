package core

import (
	"encoding/json"
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

type Store struct {
	db *bbolt.DB
}

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

func NewStore(path string) (*Store, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("opening store: %w", err)
	}

	s := &Store{db: db}
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
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s:%d", m.Name, m.Timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
}

func (s *Store) QueueEvent(e StoredEvent) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("events"))
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s:%d", e.Type, e.Timestamp.UnixNano())
		return b.Put([]byte(key), data)
	})
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
		b := tx.Bucket([]byte("metrics"))
		count = b.Stats().KeyN
		return nil
	})
	return count, err
}

func (s *Store) Close() error {
	return s.db.Close()
}
