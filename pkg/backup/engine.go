package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"

	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/recovery"
	"github.com/strata-rmm/strata-rmm-orchestrator/pkg/repository"
)

// RecoveryComponent is one independently restorable part of a backup set.
type RecoveryComponent interface {
	Backup(context.Context, io.Writer) (ComponentManifest, error)
	Restore(context.Context, io.Reader) error
	Verify(context.Context, ComponentManifest) error
}

// OperationLock is held for the full backup or restore operation.
type OperationLock interface {
	Acquire(context.Context) error
	Release(context.Context) error
}

// EngineConfig contains non-sensitive identity recorded in the manifest.
type EngineConfig struct {
	EnvironmentID string
	SourceCommit  string
	SourceRelease string
	SchemaVersion int
}

// Engine executes encrypted, externally persisted backup and recovery.
type Engine struct {
	config            EngineConfig
	repository        ArtifactRepository
	keys              KeyProvider
	quiescer          Quiescer
	lock              OperationLock
	components        map[repository.ComponentType]RecoveryComponent
	order             []repository.ComponentType
	maxComponentBytes int64
}

const defaultMaxComponentBytes int64 = 512 << 20

// NewEngine fails closed when a dependency needed for a consistent and
// recoverable backup is absent.
func NewEngine(
	config EngineConfig,
	repo ArtifactRepository,
	keys KeyProvider,
	quiescer Quiescer,
	lock OperationLock,
	components map[repository.ComponentType]RecoveryComponent,
) (*Engine, error) {
	if config.EnvironmentID == "" {
		return nil, errors.New("backup environment identity is required")
	}
	if repo == nil {
		return nil, ErrRepositoryRequired
	}
	if keys == nil {
		return nil, ErrKeyProviderRequired
	}
	if quiescer == nil {
		return nil, errors.New("quiescer is required")
	}
	if lock == nil {
		return nil, errors.New("operation lock is required")
	}
	if len(components) == 0 {
		return nil, errors.New("at least one recovery component is required")
	}
	order := []repository.ComponentType{
		repository.ComponentDatabase,
		repository.ComponentJetStream,
		repository.ComponentObjectStore,
	}
	for componentType, component := range components {
		if component == nil {
			return nil, fmt.Errorf("%s recovery component is nil", componentType)
		}
	}
	return &Engine{
		config:            config,
		repository:        repo,
		keys:              keys,
		quiescer:          quiescer,
		lock:              lock,
		components:        components,
		order:             order,
		maxComponentBytes: defaultMaxComponentBytes,
	}, nil
}

// Backup creates and atomically finalizes an encrypted external backup set.
func (e *Engine) Backup(ctx context.Context) (_ *repository.Manifest, resultErr error) {
	if err := e.lock.Acquire(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrLockNotAcquired, err)
	}
	defer func() {
		cleanupCtx, cancel := boundedCleanupContext(ctx)
		defer cancel()
		if err := e.lock.Release(cleanupCtx); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("release recovery lock: %w", err)
		}
	}()
	if err := e.quiescer.Quiesce(ctx); err != nil {
		return nil, fmt.Errorf("quiesce services: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := boundedCleanupContext(ctx)
		defer cancel()
		if err := e.quiescer.Resume(cleanupCtx); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("resume services: %w", err)
		}
	}()

	key, err := e.keys.CurrentKey(ctx)
	if err != nil {
		return nil, fmt.Errorf("resolve current recovery key: %w", err)
	}
	if err := validateRecoveryKey(key); err != nil {
		return nil, err
	}

	started := time.Now().UTC()
	backupID := uuid.NewString()
	set := repository.BackupSet{
		ID:            backupID,
		EnvironmentID: e.config.EnvironmentID,
		SourceCommit:  e.config.SourceCommit,
		SourceRelease: e.config.SourceRelease,
		SchemaVersion: e.config.SchemaVersion,
		StartedAt:     started,
	}
	if err := e.repository.CreateBackupSet(ctx, set); err != nil {
		return nil, fmt.Errorf("create external backup set: %w", err)
	}

	refs := make([]repository.ComponentRef, 0, len(e.components))
	for _, componentType := range e.order {
		component, ok := e.components[componentType]
		if !ok {
			continue
		}
		ref, err := e.backupComponent(ctx, backupID, componentType, component, key)
		if err != nil {
			return nil, err
		}
		refs = append(refs, ref)
	}
	if len(refs) != len(e.components) {
		return nil, errors.New("unsupported recovery component type")
	}

	completed := time.Now().UTC()
	manifest := &repository.Manifest{
		Version:            repository.ManifestVersion,
		BackupSetID:        backupID,
		EnvironmentID:      e.config.EnvironmentID,
		SourceCommit:       e.config.SourceCommit,
		SourceRelease:      e.config.SourceRelease,
		SchemaVersion:      e.config.SchemaVersion,
		StartedAt:          started,
		CompletedAt:        completed,
		ProtectedAt:        completed,
		RequiredComponents: refs,
		VerificationStatus: "verified",
	}
	if err := e.repository.FinalizeBackupSet(ctx, manifest); err != nil {
		return nil, fmt.Errorf("finalize external backup set: %w", err)
	}
	return manifest, nil
}

func (e *Engine) backupComponent(
	ctx context.Context,
	backupID string,
	componentType repository.ComponentType,
	component RecoveryComponent,
	key *recovery.RecoveryKey,
) (repository.ComponentRef, error) {
	var artifact bytes.Buffer
	componentManifest, err := component.Backup(ctx, &artifact)
	if err != nil {
		return repository.ComponentRef{}, fmt.Errorf("backup %s: %w", componentType, err)
	}
	if componentManifest.Type != string(componentType) {
		return repository.ComponentRef{}, fmt.Errorf("%s component returned manifest type %q", componentType, componentManifest.Type)
	}
	payload, err := json.Marshal(componentPayload{Manifest: componentManifest, Artifact: artifact.Bytes()})
	if err != nil {
		return repository.ComponentRef{}, fmt.Errorf("encode %s component payload: %w", componentType, err)
	}
	if int64(len(payload)) > e.maxComponentBytes {
		return repository.ComponentRef{}, fmt.Errorf("%s component exceeds %d-byte in-memory envelope limit", componentType, e.maxComponentBytes)
	}
	componentID := string(componentType) + ".enc"
	ad := componentAssociatedData(
		backupID,
		componentID,
		e.config.EnvironmentID,
		e.config.SourceCommit,
		e.config.SourceRelease,
		e.config.SchemaVersion,
		componentType,
		key.ID,
	)
	envelope, err := recovery.Encrypt(key.KeyMaterial, payload, ad)
	if err != nil {
		return repository.ComponentRef{}, fmt.Errorf("encrypt %s component: %w", componentType, err)
	}
	encrypted, err := envelope.Marshal()
	if err != nil {
		return repository.ComponentRef{}, fmt.Errorf("serialize %s component: %w", componentType, err)
	}
	plainDigest := sha256.Sum256(payload)
	cipherDigest := repository.DigestBase64(encrypted)
	if err := e.repository.WriteComponent(ctx, backupID, componentID, bytes.NewReader(encrypted)); err != nil {
		return repository.ComponentRef{}, fmt.Errorf("write %s component: %w", componentType, err)
	}
	if err := e.repository.FinalizeComponent(
		ctx,
		backupID,
		componentID,
		base64.StdEncoding.EncodeToString(plainDigest[:]),
		cipherDigest,
		int64(len(encrypted)),
		int64(len(payload)),
	); err != nil {
		return repository.ComponentRef{}, fmt.Errorf("finalize %s component: %w", componentType, err)
	}
	return repository.ComponentRef{
		ID:               componentID,
		Type:             componentType,
		ArtifactLoc:      componentID,
		PlaintextDigest:  base64.StdEncoding.EncodeToString(plainDigest[:]),
		CiphertextDigest: cipherDigest,
		EncryptedSize:    int64(len(encrypted)),
		OriginalSize:     int64(len(payload)),
		Encryption:       "aes-256-gcm",
		KeyID:            key.ID,
		Verified:         true,
	}, nil
}

// Restore verifies and decrypts every required component before quiescing or
// mutating the explicitly configured recovery targets.
func (e *Engine) Restore(ctx context.Context, backupID string) (resultErr error) {
	if backupID == "" {
		return ErrBackupNotFound
	}
	manifest, err := e.repository.ReadManifest(ctx, backupID)
	if err != nil {
		return fmt.Errorf("read external backup manifest: %w", err)
	}
	if err := manifest.VerifyManifest(); err != nil {
		return fmt.Errorf("verify backup manifest: %w", err)
	}
	if manifest.BackupSetID != backupID {
		return errors.New("backup manifest identity mismatch")
	}

	payloads := make(map[repository.ComponentType]componentPayload, len(manifest.RequiredComponents))
	for _, ref := range manifest.RequiredComponents {
		if _, duplicate := payloads[ref.Type]; duplicate {
			return fmt.Errorf("backup manifest contains duplicate %s component", ref.Type)
		}
		component, ok := e.components[ref.Type]
		if !ok || component == nil {
			return fmt.Errorf("required %s recovery component is unavailable", ref.Type)
		}
		payload, err := e.readComponent(ctx, manifest, ref)
		if err != nil {
			return err
		}
		payloads[ref.Type] = payload
	}
	if len(payloads) != len(e.components) {
		return errors.New("backup manifest does not contain every configured recovery component")
	}

	if err := e.lock.Acquire(ctx); err != nil {
		return fmt.Errorf("%w: %v", ErrLockNotAcquired, err)
	}
	defer func() {
		cleanupCtx, cancel := boundedCleanupContext(ctx)
		defer cancel()
		if err := e.lock.Release(cleanupCtx); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("release recovery lock: %w", err)
		}
	}()
	if err := e.quiescer.Quiesce(ctx); err != nil {
		return fmt.Errorf("quiesce services: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := boundedCleanupContext(ctx)
		defer cancel()
		if err := e.quiescer.Resume(cleanupCtx); err != nil && resultErr == nil {
			resultErr = fmt.Errorf("resume services: %w", err)
		}
	}()

	for _, componentType := range e.order {
		payload, ok := payloads[componentType]
		if !ok {
			continue
		}
		component := e.components[componentType]
		if err := component.Restore(ctx, bytes.NewReader(payload.Artifact)); err != nil {
			return fmt.Errorf("restore %s: %w", componentType, err)
		}
		if err := component.Verify(ctx, payload.Manifest); err != nil {
			return fmt.Errorf("verify restored %s: %w", componentType, err)
		}
	}
	return nil
}

// Verify validates all external artifacts without mutating recovery targets.
func (e *Engine) Verify(ctx context.Context, backupID string) error {
	manifest, err := e.repository.ReadManifest(ctx, backupID)
	if err != nil {
		return fmt.Errorf("read external backup manifest: %w", err)
	}
	if err := manifest.VerifyManifest(); err != nil {
		return fmt.Errorf("verify backup manifest: %w", err)
	}
	seen := make(map[repository.ComponentType]struct{}, len(manifest.RequiredComponents))
	for _, ref := range manifest.RequiredComponents {
		if _, duplicate := seen[ref.Type]; duplicate {
			return fmt.Errorf("backup manifest contains duplicate %s component", ref.Type)
		}
		seen[ref.Type] = struct{}{}
		if _, err := e.readComponent(ctx, manifest, ref); err != nil {
			return err
		}
	}
	return nil
}

// VerifyBackupSet validates an external backup using only the independent
// repository and key provider. It intentionally does not require source
// services, which may be unavailable during a disaster.
func VerifyBackupSet(ctx context.Context, repo ArtifactRepository, keys KeyProvider, backupID string) error {
	if repo == nil {
		return ErrRepositoryRequired
	}
	if keys == nil {
		return ErrKeyProviderRequired
	}
	engine := &Engine{repository: repo, keys: keys, maxComponentBytes: defaultMaxComponentBytes}
	return engine.Verify(ctx, backupID)
}

func (e *Engine) readComponent(
	ctx context.Context,
	manifest *repository.Manifest,
	ref repository.ComponentRef,
) (componentPayload, error) {
	if !ref.Verified || ref.Encryption != "aes-256-gcm" || ref.KeyID == "" {
		return componentPayload{}, fmt.Errorf("component %s is not finalized for authenticated recovery", ref.ID)
	}
	if err := e.repository.VerifyIntegrity(ctx, manifest.BackupSetID, ref.ID, ref.CiphertextDigest); err != nil {
		return componentPayload{}, fmt.Errorf("%w for %s: %v", ErrIntegrityCheck, ref.ID, err)
	}
	reader, err := e.repository.ReadComponent(ctx, manifest.BackupSetID, ref.ID)
	if err != nil {
		return componentPayload{}, fmt.Errorf("read component %s: %w", ref.ID, err)
	}
	if ref.EncryptedSize <= 0 || ref.EncryptedSize > e.maxComponentBytes+(1<<20) {
		_ = reader.Close()
		return componentPayload{}, fmt.Errorf("component %s encrypted size is outside supported bounds", ref.ID)
	}
	encrypted, readErr := io.ReadAll(io.LimitReader(reader, ref.EncryptedSize+1))
	closeErr := reader.Close()
	if readErr != nil {
		return componentPayload{}, fmt.Errorf("read component %s: %w", ref.ID, readErr)
	}
	if closeErr != nil {
		return componentPayload{}, fmt.Errorf("close component %s: %w", ref.ID, closeErr)
	}
	if int64(len(encrypted)) != ref.EncryptedSize {
		return componentPayload{}, fmt.Errorf("%w for component size %s", ErrIntegrityCheck, ref.ID)
	}
	if repository.DigestBase64(encrypted) != ref.CiphertextDigest {
		return componentPayload{}, fmt.Errorf("%w for %s", ErrIntegrityCheck, ref.ID)
	}
	key, err := e.keys.ResolveKey(ctx, ref.KeyID)
	if err != nil {
		return componentPayload{}, fmt.Errorf("resolve recovery key %s: %w", ref.KeyID, err)
	}
	if err := validateRecoveryKey(key); err != nil {
		return componentPayload{}, err
	}
	envelope, err := recovery.Unmarshal(encrypted)
	if err != nil {
		return componentPayload{}, fmt.Errorf("decode encrypted component %s: %w", ref.ID, err)
	}
	envelope.AssociatedData = componentAssociatedData(
		manifest.BackupSetID,
		ref.ID,
		manifest.EnvironmentID,
		manifest.SourceCommit,
		manifest.SourceRelease,
		manifest.SchemaVersion,
		ref.Type,
		ref.KeyID,
	)
	plaintext, err := recovery.Decrypt(key.KeyMaterial, envelope)
	if err != nil {
		return componentPayload{}, fmt.Errorf("decrypt component %s: %w", ref.ID, err)
	}
	digest := sha256.Sum256(plaintext)
	if base64.StdEncoding.EncodeToString(digest[:]) != ref.PlaintextDigest {
		return componentPayload{}, fmt.Errorf("%w for decrypted %s", ErrIntegrityCheck, ref.ID)
	}
	var payload componentPayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return componentPayload{}, fmt.Errorf("decode component payload %s: %w", ref.ID, err)
	}
	if payload.Manifest.Type != string(ref.Type) {
		return componentPayload{}, fmt.Errorf("component %s type mismatch", ref.ID)
	}
	if SnapshotDigest(payload.Artifact) != payload.Manifest.Digest {
		return componentPayload{}, fmt.Errorf("%w for component payload %s", ErrIntegrityCheck, ref.ID)
	}
	return payload, nil
}

func validateRecoveryKey(key *recovery.RecoveryKey) error {
	if key == nil || key.ID == "" || len(key.KeyMaterial) != 32 {
		return ErrEncryptionKeyReq
	}
	return nil
}

func componentAssociatedData(
	backupID,
	componentID,
	environmentID,
	sourceCommit,
	sourceRelease string,
	schemaVersion int,
	componentType repository.ComponentType,
	keyID string,
) []byte {
	return recovery.NewMetadataAuth().
		AppendString("backup_set_id", backupID).
		AppendString("component_id", componentID).
		AppendString("environment_id", environmentID).
		AppendString("source_commit", sourceCommit).
		AppendString("source_release", sourceRelease).
		AppendString("schema_version", fmt.Sprintf("%d", schemaVersion)).
		AppendString("component_type", string(componentType)).
		AppendString("key_id", keyID).
		Bytes()
}

type componentPayload struct {
	Manifest ComponentManifest `json:"manifest"`
	Artifact []byte            `json:"artifact"`
}

func boundedCleanupContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
}
