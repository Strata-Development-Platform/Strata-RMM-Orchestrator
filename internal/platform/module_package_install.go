package platform

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/strata-rmm/strata-rmm-orchestrator/internal/modules"
)

const modulePublisherTrustEnv = "STRATA_MODULE_PUBLISHER_KEYS"

var (
	errModulePackageBodyRequired = errors.New("signed module package body is required")
	errModuleRuntimeRootRequired = errors.New("module runtime root is not configured")
	errModulePublisherTrust      = errors.New("module publisher trust is not configured")
	errModuleEntrypointMissing   = errors.New("declared module runtime entrypoint is missing from payload")
)

type preparedModulePackage struct {
	pkg     modules.VerifiedPackage
	payload modules.ValidatedPayload
	root    string
}

func prepareSignedModulePackage(w http.ResponseWriter, r *http.Request) (preparedModulePackage, error) {
	packageBytes, err := readSignedModulePackage(w, r)
	if err != nil {
		return preparedModulePackage{}, err
	}
	root := strings.TrimSpace(os.Getenv(moduleRuntimeRootEnv))
	if root == "" {
		return preparedModulePackage{}, errModuleRuntimeRootRequired
	}
	trustRaw := strings.TrimSpace(os.Getenv(modulePublisherTrustEnv))
	if trustRaw == "" {
		return preparedModulePackage{}, errModulePublisherTrust
	}
	trust, err := modules.ParsePublisherTrustStoreJSON([]byte(trustRaw))
	if err != nil {
		return preparedModulePackage{}, fmt.Errorf("%w: %v", errModulePublisherTrust, err)
	}
	pkg, err := modules.VerifyPackage(packageBytes, trust)
	if err != nil {
		return preparedModulePackage{}, err
	}
	payload, err := modules.ValidatePayload(pkg)
	if err != nil {
		return preparedModulePackage{}, err
	}
	if err := validateSignedModuleEntrypoint(pkg.Manifest, payload); err != nil {
		return preparedModulePackage{}, err
	}
	return preparedModulePackage{pkg: pkg, payload: payload, root: root}, nil
}

func readSignedModulePackage(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	if r == nil || r.Body == nil {
		return nil, errModulePackageBodyRequired
	}
	reader := http.MaxBytesReader(w, r.Body, int64(modules.MaxPackageBytes))
	data, err := io.ReadAll(reader)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, fmt.Errorf("module package exceeds %d bytes", modules.MaxPackageBytes)
		}
		return nil, fmt.Errorf("read signed module package: %w", err)
	}
	if len(data) == 0 {
		return nil, errModulePackageBodyRequired
	}
	return data, nil
}

func validateSignedModuleEntrypoint(manifest modules.Manifest, payload modules.ValidatedPayload) error {
	if manifest.Runtime == nil {
		return nil
	}
	entrypoint := filepath.ToSlash(filepath.Clean(filepath.FromSlash(manifest.Runtime.Entrypoint)))
	for _, file := range payload.Files {
		if file.Path == entrypoint {
			return nil
		}
	}
	return errModuleEntrypointMissing
}

func writeSignedModuleInstallError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, errModulePackageBodyRequired):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "signed module package is required"})
	case errors.Is(err, errModuleRuntimeRootRequired), errors.Is(err, errModulePublisherTrust), errors.Is(err, modules.ErrUnsafeInstallRoot):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "module installation is not configured"})
	case errors.Is(err, modules.ErrUntrustedPublisher):
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "module publisher is not trusted"})
	case errors.Is(err, modules.ErrInvalidSignature):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "module package signature is invalid"})
	case errors.Is(err, modules.ErrMaterializedVersionExists), errors.Is(err, modules.ErrInstallInProgress), errors.Is(err, modules.ErrActivationInProgress):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "module package conflicts with existing runtime state"})
	case errors.Is(err, errModuleEntrypointMissing):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "module runtime entrypoint is missing from signed payload"})
	default:
		return false
	}
	return true
}
