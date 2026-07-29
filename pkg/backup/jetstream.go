package backup

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
)

var (
	ErrStreamNotFound   = errors.New("stream not found")
	ErrConsumerNotFound = errors.New("consumer not found")
)

type JetStreamBackup struct {
	ID          string             `json:"id"`
	Streams     []StreamConfig     `json:"streams"`
	Consumers   []ConsumerConfig   `json:"consumers"`
	Integrity   string             `json:"integrity"`
	Timestamp   time.Time          `json:"timestamp"`
	Version     string             `json:"version"`
}

type StreamConfig struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Subjects     []string `json:"subjects"`
	Retention    string   `json:"retention"`
	MaxConsumers int      `json:"max_consumers"`
	MaxMsgs      int64    `json:"max_msgs"`
	MaxBytes     int64    `json:"max_bytes"`
	MaxAge       int64    `json:"max_age"`
	Storage      string   `json:"storage"`
	Replicas     int      `json:"replicas"`
	Discard      string   `json:"discard"`
}

type ConsumerConfig struct {
	Stream     string `json:"stream"`
	Name       string `json:"name"`
	Durable    string `json:"durable"`
	AckPolicy  string `json:"ack_policy"`
	AckWait    int64  `json:"ack_wait"`
	MaxDeliver int    `json:"max_deliver"`
	Filter     string `json:"filter_subject"`
}

type JetStreamBackupStore struct {
	nc        *nats.Conn
	encryptor *encrypt.KeyStore
	db        *sql.DB
}

func NewJetStreamBackupStore(nc *nats.Conn, db *sql.DB, encryptor *encrypt.KeyStore) *JetStreamBackupStore {
	return &JetStreamBackupStore{nc: nc, db: db, encryptor: encryptor}
}

func (s *JetStreamBackupStore) Backup(ctx context.Context) (*JetStreamBackup, error) {
	startTime := time.Now()

	streams, err := s.listStreams(ctx)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}

	consumers, err := s.listConsumers(ctx, streams)
	if err != nil {
		return nil, fmt.Errorf("list consumers: %w", err)
	}

	backup := &JetStreamBackup{
		ID:        generateBackupID(),
		Streams:   streams,
		Consumers: consumers,
		Timestamp: startTime,
		Version:   "1.0.0",
	}

	data, err := json.Marshal(backup)
	if err != nil {
		return nil, fmt.Errorf("marshal backup: %w", err)
	}

	var encryptedData []byte
	if s.encryptor != nil {
		key, err := s.encryptor.GetActiveKey(ctx, "system")
		if err != nil {
			return nil, fmt.Errorf("get encryption key: %w", err)
		}
		encryptedData, err = s.encryptData(data, key)
		if err != nil {
			return nil, fmt.Errorf("encrypt backup: %w", err)
		}
	} else {
		encryptedData = data
	}

	digest := jsonDigest(encryptedData)
	backup.Integrity = digest

	err = s.storeBackup(ctx, backup, encryptedData)
	if err != nil {
		return nil, fmt.Errorf("store backup: %w", err)
	}

	return backup, nil
}

func (s *JetStreamBackupStore) Restore(ctx context.Context, backupID string) error {
	backup, data, err := s.loadBackup(ctx, backupID)
	if err != nil {
		return fmt.Errorf("load backup: %w", err)
	}

	err = s.verifyIntegrity(data, backup.Integrity)
	if err != nil {
		return fmt.Errorf("verify integrity: %w", err)
	}

	var decryptedData []byte
	if backup.Integrity != "none" && s.encryptor != nil {
		key, err := s.encryptor.GetActiveKey(ctx, "system")
		if err != nil {
			return fmt.Errorf("get encryption key: %w", err)
		}
		decryptedData, err = s.decryptData(data, key)
		if err != nil {
			return fmt.Errorf("decrypt backup: %w", err)
		}
	} else {
		decryptedData = data
	}

	var restoredBackup JetStreamBackup
	err = json.Unmarshal(decryptedData, &restoredBackup)
	if err != nil {
		return fmt.Errorf("unmarshal backup: %w", err)
	}

	err = s.restoreStreams(ctx, restoredBackup.Streams)
	if err != nil {
		return fmt.Errorf("restore streams: %w", err)
	}

	err = s.restoreConsumers(ctx, restoredBackup.Consumers)
	if err != nil {
		return fmt.Errorf("restore consumers: %w", err)
	}

	return nil
}

func (s *JetStreamBackupStore) ListBackups(ctx context.Context) ([]*JetStreamBackup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, integrity, timestamp, version, streams, consumers
		FROM jetstream_backups
		ORDER BY timestamp DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []*JetStreamBackup
	for rows.Next() {
		var b JetStreamBackup
		var streamsJSON, consumersJSON []byte
		err := rows.Scan(&b.ID, &b.Integrity, &b.Timestamp, &b.Version, &streamsJSON, &consumersJSON)
		if err != nil {
			continue
		}
		json.Unmarshal(streamsJSON, &b.Streams)
		json.Unmarshal(consumersJSON, &b.Consumers)
		backups = append(backups, &b)
	}
	return backups, nil
}

func (s *JetStreamBackupStore) DeleteBackup(ctx context.Context, backupID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM jetstream_backups WHERE id = $1`, backupID)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	return nil
}

func (s *JetStreamBackupStore) listStreams(ctx context.Context) ([]StreamConfig, error) {
	// Use JS API directly
	msg, err := s.nc.Request("$JS.API.STREAM.LIST", nil, time.Second*5)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}

	var resp struct {
		Type       string      `json:"type"`
		StreamInfo []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			Subjects    []string `json:"subjects"`
			Retention   string `json:"retention"`
			MaxConsumers int   `json:"max_consumers"`
			MaxMsgs     int64  `json:"max_msgs"`
			MaxBytes    int64  `json:"max_bytes"`
			MaxAge      int64  `json:"max_age"`
			Storage     string `json:"storage"`
			Replicas    int    `json:"replicas"`
			Discard     string `json:"discard"`
		} `json:"data"`
	}

	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal streams: %w", err)
	}

	var result []StreamConfig
	for _, info := range resp.StreamInfo {
		result = append(result, StreamConfig{
			Name:         info.Name,
			Description:  info.Description,
			Subjects:     info.Subjects,
			Retention:    info.Retention,
			MaxConsumers: info.MaxConsumers,
			MaxMsgs:      info.MaxMsgs,
			MaxBytes:     info.MaxBytes,
			MaxAge:       info.MaxAge,
			Storage:      info.Storage,
			Replicas:     info.Replicas,
			Discard:      info.Discard,
		})
	}
	return result, nil
}

func (s *JetStreamBackupStore) listConsumers(ctx context.Context, streams []StreamConfig) ([]ConsumerConfig, error) {
	var result []ConsumerConfig
	for _, stream := range streams {
		request := map[string]string{"stream_name": stream.Name}
		data, _ := json.Marshal(request)

		msg, err := s.nc.Request(fmt.Sprintf("$JS.API.CONSUMER.LIST.%s", stream.Name), data, time.Second*5)
		if err != nil {
			return nil, fmt.Errorf("list consumers for %s: %w", stream.Name, err)
		}

		var resp struct {
			Type       string `json:"type"`
			ConsumerInfo []struct {
				Name        string `json:"name"`
				Durable     string `json:"durable_name"`
				AckPolicy   string `json:"ack_policy"`
				AckWait     int64  `json:"ack_wait"`
				MaxDeliver  int    `json:"max_deliver"`
				Filter      string `json:"filter_subject"`
			} `json:"data"`
		}

		if err := json.Unmarshal(msg.Data, &resp); err != nil {
			continue
		}

		for _, info := range resp.ConsumerInfo {
			result = append(result, ConsumerConfig{
				Stream:     stream.Name,
				Name:       info.Name,
				Durable:    info.Durable,
				AckPolicy:  info.AckPolicy,
				AckWait:    info.AckWait,
				MaxDeliver: info.MaxDeliver,
				Filter:     info.Filter,
			})
		}
	}
	return result, nil
}

func (s *JetStreamBackupStore) restoreStreams(ctx context.Context, streams []StreamConfig) error {
	for _, stream := range streams {
		config := map[string]interface{}{
			"name":        stream.Name,
			"description": stream.Description,
			"subjects":    stream.Subjects,
			"retention":   stream.Retention,
			"max_consumers": stream.MaxConsumers,
			"max_msgs":    stream.MaxMsgs,
			"max_bytes":   stream.MaxBytes,
			"max_age":     stream.MaxAge,
			"storage":     stream.Storage,
			"replicas":    stream.Replicas,
			"discard":     stream.Discard,
		}

		data, _ := json.Marshal(config)
		_, err := s.nc.Request("$JS.API.STREAM.CREATE", data, time.Second*5)
		if err != nil {
			return fmt.Errorf("create stream %s: %w", stream.Name, err)
		}
	}
	return nil
}

func (s *JetStreamBackupStore) restoreConsumers(ctx context.Context, consumers []ConsumerConfig) error {
	for _, consumer := range consumers {
		config := map[string]interface{}{
			"stream_name": consumer.Stream,
			"durable":     consumer.Durable,
			"ack_policy":  consumer.AckPolicy,
			"ack_wait":    consumer.AckWait,
			"max_deliver": consumer.MaxDeliver,
			"filter_subject": consumer.Filter,
		}

		data, _ := json.Marshal(config)
		_, err := s.nc.Request("$JS.API.CONSUMER.CREATE", data, time.Second*5)
		if err != nil {
			return fmt.Errorf("create consumer %s: %w", consumer.Name, err)
		}
	}
	return nil
}

func (s *JetStreamBackupStore) encryptData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	ciphertext, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	return []byte(ciphertext), nil
}

func (s *JetStreamBackupStore) decryptData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	plaintext, err := encryptor.Decrypt(string(data))
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func (s *JetStreamBackupStore) verifyIntegrity(data []byte, expectedDigest string) error {
	actualDigest := jsonDigest(data)
	if actualDigest != expectedDigest {
		return fmt.Errorf("%w: expected %s, got %s", ErrBackupCorrupted, expectedDigest, actualDigest)
	}
	return nil
}

func (s *JetStreamBackupStore) storeBackup(ctx context.Context, backup *JetStreamBackup, data []byte) error {
	streamsJSON, _ := json.Marshal(backup.Streams)
	consumersJSON, _ := json.Marshal(backup.Consumers)

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jetstream_backups (id, integrity, timestamp, version, streams, consumers, data)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, backup.ID, backup.Integrity, backup.Timestamp, backup.Version, streamsJSON, consumersJSON, data)
	if err != nil {
		return fmt.Errorf("store backup: %w", err)
	}
	return nil
}

func (s *JetStreamBackupStore) loadBackup(ctx context.Context, backupID string) (*JetStreamBackup, []byte, error) {
	var backup JetStreamBackup
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, integrity, timestamp, version, streams, consumers, data
		FROM jetstream_backups WHERE id = $1
	`, backupID).Scan(
		&backup.ID, &backup.Integrity, &backup.Timestamp, &backup.Version,
		&backup.Streams, &backup.Consumers, &data,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load backup: %w", err)
	}
	return &backup, data, nil
}

func jsonDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return base64.StdEncoding.EncodeToString(digest[:])
}
