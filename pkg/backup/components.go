package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"github.com/nats-io/nats.go"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/storage"
)

// ComponentManifest describes the observable contents of a component artifact.
// Digest is the SHA-256 digest of the unencrypted component artifact.
type ComponentManifest struct {
	Type        string            `json:"type"`
	Count       int               `json:"count"`
	Size        int64             `json:"size"`
	Digest      string            `json:"digest"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	ObjectIndex []ObjectManifest  `json:"objects,omitempty"`
}

// ObjectManifest describes one object stored in an object-storage artifact.
type ObjectManifest struct {
	Key         string            `json:"key"`
	Size        int64             `json:"size"`
	Digest      string            `json:"digest"`
	ContentType string            `json:"content_type,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type jetStreamSnapshot struct {
	Streams []jetStreamStream `json:"streams"`
}

type jetStreamStream struct {
	Config    nats.StreamConfig   `json:"config"`
	Consumers []jetStreamConsumer `json:"consumers"`
	Messages  []jetStreamMessage  `json:"messages"`
}

type jetStreamConsumer struct {
	Config    nats.ConsumerConfig `json:"config"`
	Delivered nats.SequenceInfo   `json:"delivered"`
	AckFloor  nats.SequenceInfo   `json:"ack_floor"`
}

type jetStreamMessage struct {
	Subject string      `json:"subject"`
	Header  nats.Header `json:"header,omitempty"`
	Data    []byte      `json:"data"`
}

// JetStreamRecovery backs up and restores streams, messages, and durable
// consumer configurations through a real NATS connection.
type JetStreamRecovery struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

// NewJetStreamRecovery creates a JetStream recovery component.
func NewJetStreamRecovery(nc *nats.Conn) (*JetStreamRecovery, error) {
	if nc == nil || !nc.IsConnected() {
		return nil, errors.New("connected NATS client is required")
	}
	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("create JetStream context: %w", err)
	}
	if _, err := js.AccountInfo(); err != nil {
		return nil, fmt.Errorf("JetStream unavailable: %w", err)
	}
	return &JetStreamRecovery{nc: nc, js: js}, nil
}

// Backup writes a deterministic JSON snapshot to w.
func (r *JetStreamRecovery) Backup(ctx context.Context, w io.Writer) (ComponentManifest, error) {
	if w == nil {
		return ComponentManifest{}, errors.New("backup writer is required")
	}
	snapshot := jetStreamSnapshot{}
	infoCh := r.js.StreamsInfo(nats.Context(ctx))
	for info := range infoCh {
		if info == nil {
			return ComponentManifest{}, errors.New("JetStream stream enumeration returned nil stream")
		}
		entry := jetStreamStream{Config: info.Config}
		consumerCh := r.js.ConsumersInfo(info.Config.Name, nats.Context(ctx))
		for consumer := range consumerCh {
			if consumer == nil {
				return ComponentManifest{}, fmt.Errorf("consumer enumeration failed for stream %s", info.Config.Name)
			}
			entry.Consumers = append(entry.Consumers, jetStreamConsumer{
				Config:    consumer.Config,
				Delivered: consumer.Delivered,
				AckFloor:  consumer.AckFloor,
			})
		}
		for seq := info.State.FirstSeq; seq != 0 && seq <= info.State.LastSeq; seq++ {
			msg, err := r.js.GetMsg(info.Config.Name, seq, nats.Context(ctx))
			if err != nil {
				if errors.Is(err, nats.ErrMsgNotFound) {
					continue
				}
				return ComponentManifest{}, fmt.Errorf("read stream %s sequence %d: %w", info.Config.Name, seq, err)
			}
			entry.Messages = append(entry.Messages, jetStreamMessage{
				Subject: msg.Subject,
				Header:  msg.Header,
				Data:    append([]byte(nil), msg.Data...),
			})
		}
		sort.Slice(entry.Consumers, func(i, j int) bool {
			return entry.Consumers[i].Config.Durable < entry.Consumers[j].Config.Durable
		})
		snapshot.Streams = append(snapshot.Streams, entry)
	}
	sort.Slice(snapshot.Streams, func(i, j int) bool {
		return snapshot.Streams[i].Config.Name < snapshot.Streams[j].Config.Name
	})
	data, err := json.Marshal(snapshot)
	if err != nil {
		return ComponentManifest{}, fmt.Errorf("marshal JetStream snapshot: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return ComponentManifest{}, fmt.Errorf("write JetStream snapshot: %w", err)
	}
	messageCount := 0
	for _, stream := range snapshot.Streams {
		messageCount += len(stream.Messages)
	}
	digest := sha256.Sum256(data)
	return ComponentManifest{
		Type:   string(repositoryComponentJetStream),
		Count:  messageCount,
		Size:   int64(len(data)),
		Digest: hex.EncodeToString(digest[:]),
		Metadata: map[string]string{
			"streams": strconv.Itoa(len(snapshot.Streams)),
		},
	}, nil
}

// Restore reconciles the snapshot into the connected JetStream account.
// An existing non-empty stream is accepted only when every archived message is
// already present. This makes an interrupted retry idempotent without silently
// appending duplicates or overwriting divergent live data.
func (r *JetStreamRecovery) Restore(ctx context.Context, source io.Reader) error {
	var snapshot jetStreamSnapshot
	if err := json.NewDecoder(source).Decode(&snapshot); err != nil {
		return fmt.Errorf("decode JetStream snapshot: %w", err)
	}
	for _, stream := range snapshot.Streams {
		info, err := r.js.StreamInfo(stream.Config.Name, nats.Context(ctx))
		publishMessages := true
		switch {
		case err == nil:
			if _, err := r.js.UpdateStream(&stream.Config, nats.Context(ctx)); err != nil {
				return fmt.Errorf("update stream %s: %w", stream.Config.Name, err)
			}
			if info.State.Msgs != 0 {
				matches, err := r.streamMatches(ctx, stream, info)
				if err != nil {
					return err
				}
				if !matches {
					return fmt.Errorf("target stream %s contains data different from the backup", stream.Config.Name)
				}
				publishMessages = false
			}
		case errors.Is(err, nats.ErrStreamNotFound):
			if _, err := r.js.AddStream(&stream.Config, nats.Context(ctx)); err != nil {
				return fmt.Errorf("create stream %s: %w", stream.Config.Name, err)
			}
		default:
			return fmt.Errorf("inspect stream %s: %w", stream.Config.Name, err)
		}
		if publishMessages {
			for _, archived := range stream.Messages {
				msg := &nats.Msg{
					Subject: archived.Subject,
					Header:  archived.Header,
					Data:    archived.Data,
				}
				if _, err := r.js.PublishMsg(msg, nats.Context(ctx)); err != nil {
					return fmt.Errorf("restore message to %s: %w", stream.Config.Name, err)
				}
			}
		}
		for _, consumer := range stream.Consumers {
			config := consumer.Config
			if consumer.AckFloor.Stream > 0 {
				config.DeliverPolicy = nats.DeliverByStartSequencePolicy
				config.OptStartSeq = consumer.AckFloor.Stream + 1
			}
			if _, err := r.js.AddConsumer(stream.Config.Name, &config, nats.Context(ctx)); err != nil &&
				!strings.Contains(strings.ToLower(err.Error()), "already in use") {
				return fmt.Errorf("restore consumer %s/%s: %w", stream.Config.Name, config.Durable, err)
			}
		}
	}
	return nil
}

func (r *JetStreamRecovery) streamMatches(ctx context.Context, archived jetStreamStream, info *nats.StreamInfo) (bool, error) {
	if info.State.Msgs != uint64(len(archived.Messages)) {
		return false, nil
	}
	index := 0
	for seq := info.State.FirstSeq; seq != 0 && seq <= info.State.LastSeq; seq++ {
		msg, err := r.js.GetMsg(archived.Config.Name, seq, nats.Context(ctx))
		if err != nil {
			if errors.Is(err, nats.ErrMsgNotFound) {
				continue
			}
			return false, fmt.Errorf("inspect existing stream %s sequence %d: %w", archived.Config.Name, seq, err)
		}
		if index >= len(archived.Messages) {
			return false, nil
		}
		expected := archived.Messages[index]
		if msg.Subject != expected.Subject || !bytes.Equal(msg.Data, expected.Data) || !reflect.DeepEqual(msg.Header, expected.Header) {
			return false, nil
		}
		index++
	}
	return index == len(archived.Messages), nil
}

// Verify compares the live stream and message counts with the manifest.
func (r *JetStreamRecovery) Verify(ctx context.Context, manifest ComponentManifest) error {
	if manifest.Type != string(repositoryComponentJetStream) {
		return fmt.Errorf("unexpected component type %q", manifest.Type)
	}
	streamCount, messageCount := 0, 0
	for info := range r.js.StreamsInfo(nats.Context(ctx)) {
		if info == nil {
			return errors.New("JetStream verification returned nil stream")
		}
		streamCount++
		messageCount += int(info.State.Msgs)
	}
	if want, err := strconv.Atoi(manifest.Metadata["streams"]); err != nil || want != streamCount {
		return fmt.Errorf("JetStream stream count mismatch: got %d, want %s", streamCount, manifest.Metadata["streams"])
	}
	if messageCount != manifest.Count {
		return fmt.Errorf("JetStream message count mismatch: got %d, want %d", messageCount, manifest.Count)
	}
	return nil
}

type componentType string

const (
	repositoryComponentJetStream componentType = "jetstream"
	repositoryComponentObjects   componentType = "object_storage"
)

// ObjectStorageRecovery backs up actual object bytes and metadata using the
// repository's storage.Backend abstraction.
type ObjectStorageRecovery struct {
	source storage.Backend
	target storage.Backend
}

// NewObjectStorageRecovery creates an object-storage recovery component.
func NewObjectStorageRecovery(source, target storage.Backend) (*ObjectStorageRecovery, error) {
	if source == nil {
		return nil, errors.New("source object-storage backend is required")
	}
	if target == nil {
		return nil, errors.New("target object-storage backend is required")
	}
	return &ObjectStorageRecovery{source: source, target: target}, nil
}

// Backup writes a tar stream containing exact object bytes and metadata.
func (r *ObjectStorageRecovery) Backup(ctx context.Context, w io.Writer) (ComponentManifest, error) {
	if w == nil {
		return ComponentManifest{}, errors.New("backup writer is required")
	}
	hasher := sha256.New()
	counting := &countingWriter{writer: io.MultiWriter(w, hasher)}
	tw := tar.NewWriter(counting)
	objects, err := r.source.List(ctx, "", storage.ListOptions{MaxKeys: 1000})
	if err != nil {
		return ComponentManifest{}, fmt.Errorf("list source objects: %w", err)
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })
	manifest := ComponentManifest{Type: string(repositoryComponentObjects)}
	for _, object := range objects {
		if object.Key == "" || strings.Contains(object.Key, "\x00") {
			return ComponentManifest{}, fmt.Errorf("unsafe object key %q", object.Key)
		}
		object, err = r.source.Stat(ctx, object.Key)
		if err != nil {
			return ComponentManifest{}, fmt.Errorf("stat source object %s: %w", object.Key, err)
		}
		reader, err := r.source.Download(ctx, object.Key)
		if err != nil {
			return ComponentManifest{}, fmt.Errorf("download source object %s: %w", object.Key, err)
		}
		objectHasher := sha256.New()
		header := &tar.Header{
			Name:       object.Key,
			Mode:       0600,
			Size:       object.Size,
			ModTime:    object.LastModified,
			PAXRecords: map[string]string{"strata.content_type": object.ContentType},
		}
		for key, value := range object.Metadata {
			header.PAXRecords["strata.meta."+key] = value
		}
		if err := tw.WriteHeader(header); err != nil {
			reader.Close()
			return ComponentManifest{}, fmt.Errorf("write object header %s: %w", object.Key, err)
		}
		written, copyErr := io.Copy(io.MultiWriter(tw, objectHasher), reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return ComponentManifest{}, fmt.Errorf("archive object %s: %w", object.Key, copyErr)
		}
		if closeErr != nil {
			return ComponentManifest{}, fmt.Errorf("close source object %s: %w", object.Key, closeErr)
		}
		if written != object.Size {
			return ComponentManifest{}, fmt.Errorf("object %s size changed during backup: got %d, want %d", object.Key, written, object.Size)
		}
		manifest.ObjectIndex = append(manifest.ObjectIndex, ObjectManifest{
			Key:         object.Key,
			Size:        object.Size,
			Digest:      hex.EncodeToString(objectHasher.Sum(nil)),
			ContentType: object.ContentType,
			Metadata:    object.Metadata,
		})
	}
	if err := tw.Close(); err != nil {
		return ComponentManifest{}, fmt.Errorf("finalize object archive: %w", err)
	}
	manifest.Count = len(manifest.ObjectIndex)
	manifest.Size = counting.count
	manifest.Digest = hex.EncodeToString(hasher.Sum(nil))
	return manifest, nil
}

// Restore streams objects from an archive into the target backend.
func (r *ObjectStorageRecovery) Restore(ctx context.Context, source io.Reader) error {
	tr := tar.NewReader(source)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read object archive: %w", err)
		}
		if header.Name == "" || strings.Contains(header.Name, "\x00") || header.Size < 0 {
			return fmt.Errorf("unsafe object archive entry %q", header.Name)
		}
		metadata := map[string]string{}
		contentType := header.PAXRecords["strata.content_type"]
		for key, value := range header.PAXRecords {
			if strings.HasPrefix(key, "strata.meta.") {
				metadata[strings.TrimPrefix(key, "strata.meta.")] = value
			}
		}
		if _, err := r.target.Upload(ctx, header.Name, io.LimitReader(tr, header.Size), storage.UploadOptions{
			ContentType:   contentType,
			Metadata:      metadata,
			ContentLength: header.Size,
		}); err != nil {
			return fmt.Errorf("restore object %s: %w", header.Name, err)
		}
	}
}

// Verify checks target object sizes, metadata, and content digests.
func (r *ObjectStorageRecovery) Verify(ctx context.Context, manifest ComponentManifest) error {
	if manifest.Type != string(repositoryComponentObjects) {
		return fmt.Errorf("unexpected component type %q", manifest.Type)
	}
	for _, expected := range manifest.ObjectIndex {
		info, err := r.target.Stat(ctx, expected.Key)
		if err != nil {
			return fmt.Errorf("stat restored object %s: %w", expected.Key, err)
		}
		if info.Size != expected.Size || info.ContentType != expected.ContentType || !reflect.DeepEqual(info.Metadata, expected.Metadata) {
			return fmt.Errorf("restored object %s metadata mismatch", expected.Key)
		}
		reader, err := r.target.Download(ctx, expected.Key)
		if err != nil {
			return fmt.Errorf("read restored object %s: %w", expected.Key, err)
		}
		hasher := sha256.New()
		_, copyErr := io.Copy(hasher, reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return fmt.Errorf("hash restored object %s: %w", expected.Key, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("close restored object %s: %w", expected.Key, closeErr)
		}
		if digest := hex.EncodeToString(hasher.Sum(nil)); digest != expected.Digest {
			return fmt.Errorf("restored object %s digest mismatch", expected.Key)
		}
	}
	return nil
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (w *countingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.count += int64(n)
	return n, err
}

// SnapshotDigest returns the digest of a component artifact.
func SnapshotDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// DecodeComponentManifest is a small helper used by component tests and
// callers that persist manifests alongside encrypted artifacts.
func DecodeComponentManifest(data []byte) (ComponentManifest, error) {
	var manifest ComponentManifest
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&manifest); err != nil {
		return ComponentManifest{}, err
	}
	return manifest, nil
}
