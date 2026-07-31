package backup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/recovery"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/repository"
)

func TestEngineBackupRestoreRoundTrip(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo, err := repository.NewFilesystemRepository(filepath.Join(root, "repository"))
	require.NoError(t, err)
	keys, err := recovery.NewFileKeyProvider(filepath.Join(root, "keys"))
	require.NoError(t, err)
	_, err = keys.RotateKey(ctx, "test")
	require.NoError(t, err)

	component := &memoryRecoveryComponent{source: []byte("tenant-state")}
	quiescer := &recordingQuiescer{}
	lock := &recordingLock{}
	engine, err := NewEngine(
		EngineConfig{EnvironmentID: "test", SourceCommit: "abc123", SourceRelease: "test", SchemaVersion: 64},
		repo,
		keys,
		quiescer,
		lock,
		map[repository.ComponentType]RecoveryComponent{repository.ComponentDatabase: component},
	)
	require.NoError(t, err)

	manifest, err := engine.Backup(ctx)
	require.NoError(t, err)
	require.NotEmpty(t, manifest.BackupSetID)
	require.Equal(t, "verified", manifest.VerificationStatus)
	require.Equal(t, 1, quiescer.quiesceCalls)
	require.Equal(t, 1, quiescer.resumeCalls)
	require.Equal(t, 1, lock.acquireCalls)
	require.Equal(t, 1, lock.releaseCalls)

	component.source = nil
	require.NoError(t, engine.Restore(ctx, manifest.BackupSetID))
	require.Equal(t, []byte("tenant-state"), component.restored)
	require.True(t, component.verified)
	require.Equal(t, 2, quiescer.quiesceCalls)
	require.Equal(t, 2, quiescer.resumeCalls)
	require.Equal(t, 2, lock.acquireCalls)
	require.Equal(t, 2, lock.releaseCalls)
}

func TestEngineRejectsTamperingBeforeMutation(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repositoryRoot := filepath.Join(root, "repository")
	repo, err := repository.NewFilesystemRepository(repositoryRoot)
	require.NoError(t, err)
	keys, err := recovery.NewFileKeyProvider(filepath.Join(root, "keys"))
	require.NoError(t, err)
	_, err = keys.RotateKey(ctx, "test")
	require.NoError(t, err)
	component := &memoryRecoveryComponent{source: []byte("sensitive-state")}
	quiescer := &recordingQuiescer{}
	lock := &recordingLock{}
	engine, err := NewEngine(
		EngineConfig{EnvironmentID: "test"},
		repo,
		keys,
		quiescer,
		lock,
		map[repository.ComponentType]RecoveryComponent{repository.ComponentDatabase: component},
	)
	require.NoError(t, err)
	manifest, err := engine.Backup(ctx)
	require.NoError(t, err)

	path := filepath.Join(repositoryRoot, manifest.BackupSetID, "components", "postgresql.enc")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	data[len(data)-1] ^= 0xff
	require.NoError(t, os.WriteFile(path, data, 0600))

	quiescer.quiesceCalls = 0
	lock.acquireCalls = 0
	err = engine.Restore(ctx, manifest.BackupSetID)
	require.ErrorIs(t, err, ErrIntegrityCheck)
	require.Zero(t, quiescer.quiesceCalls, "integrity must be checked before quiescing")
	require.Zero(t, lock.acquireCalls, "integrity must be checked before target mutation")
	require.Empty(t, component.restored)
}

func TestEngineAlwaysResumesAndUnlocksAfterFailure(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	repo, err := repository.NewFilesystemRepository(filepath.Join(root, "repository"))
	require.NoError(t, err)
	keys, err := recovery.NewFileKeyProvider(filepath.Join(root, "keys"))
	require.NoError(t, err)
	_, err = keys.RotateKey(ctx, "test")
	require.NoError(t, err)
	quiescer := &recordingQuiescer{}
	lock := &recordingLock{}
	engine, err := NewEngine(
		EngineConfig{EnvironmentID: "test"},
		repo,
		keys,
		quiescer,
		lock,
		map[repository.ComponentType]RecoveryComponent{
			repository.ComponentDatabase: &memoryRecoveryComponent{backupErr: errors.New("injected")},
		},
	)
	require.NoError(t, err)

	_, err = engine.Backup(ctx)
	require.ErrorContains(t, err, "injected")
	require.Equal(t, 1, quiescer.resumeCalls)
	require.Equal(t, 1, lock.releaseCalls)
}

type memoryRecoveryComponent struct {
	source    []byte
	restored  []byte
	verified  bool
	backupErr error
}

func (c *memoryRecoveryComponent) Backup(_ context.Context, writer io.Writer) (ComponentManifest, error) {
	if c.backupErr != nil {
		return ComponentManifest{}, c.backupErr
	}
	if _, err := writer.Write(c.source); err != nil {
		return ComponentManifest{}, err
	}
	return ComponentManifest{
		Type:   string(repository.ComponentDatabase),
		Count:  1,
		Size:   int64(len(c.source)),
		Digest: SnapshotDigest(c.source),
	}, nil
}

func (c *memoryRecoveryComponent) Restore(_ context.Context, reader io.Reader) error {
	data, err := io.ReadAll(reader)
	if err == nil {
		c.restored = data
	}
	return err
}

func (c *memoryRecoveryComponent) Verify(_ context.Context, manifest ComponentManifest) error {
	if !bytes.Equal(c.restored, c.source) && c.source != nil {
		return errors.New("restored data mismatch")
	}
	if SnapshotDigest(c.restored) != manifest.Digest {
		return errors.New("restored digest mismatch")
	}
	c.verified = true
	return nil
}

type recordingQuiescer struct {
	quiesceCalls int
	resumeCalls  int
}

func (q *recordingQuiescer) Quiesce(context.Context) error {
	q.quiesceCalls++
	return nil
}

func (q *recordingQuiescer) Resume(context.Context) error {
	q.resumeCalls++
	return nil
}

func (q *recordingQuiescer) Status(context.Context) (QuiesceStatus, error) {
	return QuiesceStatus{Quiesced: q.quiesceCalls > q.resumeCalls}, nil
}

type recordingLock struct {
	acquireCalls int
	releaseCalls int
}

func (l *recordingLock) Acquire(context.Context) error {
	l.acquireCalls++
	return nil
}

func (l *recordingLock) Release(context.Context) error {
	l.releaseCalls++
	return nil
}
