package backup

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/encrypt"
)

var (
	ErrStreamNotFound      = errors.New("stream not found")
	ErrConsumerNotFound    = errors.New("consumer not found")
	ErrJetStreamNotEnabled = errors.New("JetStream not enabled on NATS connection")
)

type JetStreamBackup struct {
	ID        string           `json:"id"`
	Streams   []StreamConfig   `json:"streams"`
	Consumers []ConsumerConfig `json:"consumers"`
	Messages  []MessageBackup  `json:"messages,omitempty"`
	Integrity string           `json:"integrity"`
	Timestamp time.Time        `json:"timestamp"`
	Version   string           `json:"version"`
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

type MessageBackup struct {
	Stream  string `json:"stream"`
	Subject string `json:"subject"`
	Data    string `json:"data"`
	Seq     uint64 `json:"seq"`
}

type JetStreamBackupStore struct {
	nc        *nats.Conn
	js        jetstream.JetStream
	encryptor *encrypt.KeyStore
	db        *sql.DB
}

func NewJetStreamBackupStore(nc *nats.Conn, db *sql.DB, encryptor *encrypt.KeyStore) (*JetStreamBackupStore, error) {
	if nc == nil {
		return nil, fmt.Errorf("nats connection is nil")
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrJetStreamNotEnabled, err)
	}
	return &JetStreamBackupStore{nc: nc, js: js, db: db, encryptor: encryptor}, nil
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

	messages, err := s.backupMessages(ctx, streams)
	if err != nil {
		return nil, fmt.Errorf("backup messages: %w", err)
	}

	backup := &JetStreamBackup{
		ID:        generateJetStreamBackupID(),
		Streams:   streams,
		Consumers: consumers,
		Messages:  messages,
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
		encryptedData, err = encryptJetStreamData(data, key)
		if err != nil {
			return nil, fmt.Errorf("encrypt backup: %w", err)
		}
	} else {
		encryptedData = data
	}

	digest := sha256.Sum256(encryptedData)
	backup.Integrity = base64.StdEncoding.EncodeToString(digest[:])

	err = s.storeJetStreamBackup(ctx, backup, encryptedData)
	if err != nil {
		return nil, fmt.Errorf("store backup: %w", err)
	}

	return backup, nil
}

func (s *JetStreamBackupStore) Restore(ctx context.Context, backupID string) error {
	backup, data, err := s.loadJetStreamBackup(ctx, backupID)
	if err != nil {
		return fmt.Errorf("load backup: %w", err)
	}

	err = verifyJetStreamIntegrity(data, backup.Integrity)
	if err != nil {
		return fmt.Errorf("verify integrity: %w", err)
	}

	var decryptedData []byte
	if backup.Integrity != "" && s.encryptor != nil {
		key, err := s.encryptor.GetActiveKey(ctx, "system")
		if err != nil {
			return fmt.Errorf("get encryption key: %w", err)
		}
		decryptedData, err = decryptJetStreamData(data, key)
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

	err = s.restoreMessages(ctx, restoredBackup.Messages)
	if err != nil {
		return fmt.Errorf("restore messages: %w", err)
	}

	return nil
}

func (s *JetStreamBackupStore) ListBackups(ctx context.Context) ([]*JetStreamBackup, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, integrity, timestamp, version, data
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
		var data []byte
		err := rows.Scan(&b.ID, &b.Integrity, &b.Timestamp, &b.Version, &data)
		if err != nil {
			return nil, fmt.Errorf("scan backup row: %w", err)
		}
		json.Unmarshal(data, &b)
		backups = append(backups, &b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration: %w", err)
	}
	return backups, nil
}

func (s *JetStreamBackupStore) DeleteBackup(ctx context.Context, backupID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM jetstream_backups WHERE id = $1`, backupID)
	if err != nil {
		return fmt.Errorf("delete backup: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("backup not found: %s", backupID)
	}
	return nil
}

func (s *JetStreamBackupStore) listStreams(ctx context.Context) ([]StreamConfig, error) {
	streamLister := s.js.ListStreams(ctx)
	var result []StreamConfig
	for info := range streamLister.Info() {
		if streamLister.Err() != nil {
			return nil, fmt.Errorf("list streams: %w", streamLister.Err())
		}
		cfg := info.Config
		age := int64(0)
		if cfg.MaxAge > 0 {
			age = cfg.MaxAge.Nanoseconds()
		}
		result = append(result, StreamConfig{
			Name:         cfg.Name,
			Description:  cfg.Description,
			Subjects:     cfg.Subjects,
			Retention:    cfg.Retention.String(),
			MaxConsumers: cfg.MaxConsumers,
			MaxMsgs:      cfg.MaxMsgs,
			MaxBytes:     cfg.MaxBytes,
			MaxAge:       age,
			Storage:      cfg.Storage.String(),
			Replicas:     cfg.Replicas,
			Discard:      cfg.Discard.String(),
		})
	}
	return result, nil
}

func (s *JetStreamBackupStore) listConsumers(ctx context.Context, streams []StreamConfig) ([]ConsumerConfig, error) {
	var result []ConsumerConfig
	for _, stream := range streams {
		str, err := s.js.Stream(ctx, stream.Name)
		if err != nil {
			return nil, fmt.Errorf("get stream %s: %w", stream.Name, err)
		}

		consumerLister := str.ListConsumers(ctx)
		for info := range consumerLister.Info() {
			if consumerLister.Err() != nil {
				continue
			}
			cfg := info.Config
			wait := int64(0)
			if cfg.AckWait > 0 {
				wait = cfg.AckWait.Nanoseconds()
			}
			result = append(result, ConsumerConfig{
				Stream:     stream.Name,
				Name:       cfg.Name,
				Durable:    cfg.Durable,
				AckPolicy:  cfg.AckPolicy.String(),
				AckWait:    wait,
				MaxDeliver: cfg.MaxDeliver,
				Filter:     cfg.FilterSubject,
			})
		}
	}
	return result, nil
}

func (s *JetStreamBackupStore) backupMessages(ctx context.Context, streams []StreamConfig) ([]MessageBackup, error) {
	var result []MessageBackup
	for _, stream := range streams {
		str, err := s.js.Stream(ctx, stream.Name)
		if err != nil {
			continue
		}

		sub, err := str.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{})
		if err != nil {
			continue
		}

		for i := 0; i < 10; i++ {
			batch, err := sub.Fetch(100)
			if err != nil {
				break
			}
			for msg := range batch.Messages() {
				meta, err := msg.Metadata()
				if err != nil {
					continue
				}
				result = append(result, MessageBackup{
					Stream:  stream.Name,
					Subject: msg.Subject(),
					Data:    base64.StdEncoding.EncodeToString(msg.Data()),
					Seq:     meta.Sequence.Stream,
				})
			}
			if batch.Error() != nil {
				break
			}
			if len(batch.Messages()) == 0 {
				break
			}
		}
	}
	return result, nil
}

func (s *JetStreamBackupStore) restoreStreams(ctx context.Context, streams []StreamConfig) error {
	for _, stream := range streams {
		_, err := s.js.CreateStream(ctx, jetstream.StreamConfig{
			Name:         stream.Name,
			Description:  stream.Description,
			Subjects:     stream.Subjects,
			MaxConsumers: stream.MaxConsumers,
			MaxMsgs:      stream.MaxMsgs,
			MaxBytes:     stream.MaxBytes,
			Storage:      parseJetStreamStorage(stream.Storage),
			Replicas:     stream.Replicas,
		})
		if err != nil {
			return fmt.Errorf("create stream %s: %w", stream.Name, err)
		}
	}
	return nil
}

func (s *JetStreamBackupStore) restoreConsumers(ctx context.Context, consumers []ConsumerConfig) error {
	for _, consumer := range consumers {
		_, err := s.js.CreateOrUpdateConsumer(ctx, consumer.Stream, jetstream.ConsumerConfig{
			Name:          consumer.Name,
			Durable:       consumer.Durable,
			AckPolicy:     parseJetStreamAckPolicy(consumer.AckPolicy),
			MaxDeliver:    consumer.MaxDeliver,
			FilterSubject: consumer.Filter,
		})
		if err != nil {
			return fmt.Errorf("create consumer %s on %s: %w", consumer.Name, consumer.Stream, err)
		}
	}
	return nil
}

func (s *JetStreamBackupStore) restoreMessages(ctx context.Context, messages []MessageBackup) error {
	for _, msg := range messages {
		data, err := base64.StdEncoding.DecodeString(msg.Data)
		if err != nil {
			continue
		}

		_, err = s.js.Publish(ctx, msg.Subject, data)
		if err != nil {
			return fmt.Errorf("publish message to %s: %w", msg.Subject, err)
		}
	}
	return nil
}

func encryptJetStreamData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	ciphertext, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("encrypt: %w", err)
	}
	return []byte(ciphertext), nil
}

func decryptJetStreamData(data []byte, key *encrypt.TenantKey) ([]byte, error) {
	encryptor := encrypt.NewEncryptor(key)
	plaintext, err := encryptor.Decrypt(string(data))
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

func verifyJetStreamIntegrity(data []byte, expectedDigest string) error {
	digest := sha256.Sum256(data)
	actualDigest := base64.StdEncoding.EncodeToString(digest[:])
	if actualDigest != expectedDigest {
		return fmt.Errorf("%w: expected %s, got %s", ErrBackupCorrupted, expectedDigest, actualDigest)
	}
	return nil
}

func (s *JetStreamBackupStore) storeJetStreamBackup(ctx context.Context, backup *JetStreamBackup, data []byte) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO jetstream_backups (id, integrity, timestamp, version, data)
		VALUES ($1, $2, $3, $4, $5)
	`, backup.ID, backup.Integrity, backup.Timestamp, backup.Version, data)
	return err
}

func (s *JetStreamBackupStore) loadJetStreamBackup(ctx context.Context, backupID string) (*JetStreamBackup, []byte, error) {
	var backup JetStreamBackup
	var data []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT id, integrity, timestamp, version, data
		FROM jetstream_backups WHERE id = $1
	`, backupID).Scan(
		&backup.ID, &backup.Integrity, &backup.Timestamp, &backup.Version, &data,
	)
	if err != nil {
		return nil, nil, fmt.Errorf("load backup: %w", err)
	}
	return &backup, data, nil
}

func parseJetStreamStorage(s string) jetstream.StorageType {
	switch s {
	case "memory":
		return jetstream.MemoryStorage
	default:
		return jetstream.FileStorage
	}
}

func parseJetStreamAckPolicy(s string) jetstream.AckPolicy {
	switch s {
	case "all":
		return jetstream.AckAllPolicy
	case "none":
		return jetstream.AckNonePolicy
	default:
		return jetstream.AckExplicitPolicy
	}
}

func generateJetStreamBackupID() string {
	id := make([]byte, 16)
	rand.Read(id)
	return "jsbackup_" + base64.URLEncoding.EncodeToString(id)
}
