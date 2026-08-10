package modules

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
)

const (
	maxPayloadFiles         = 1024
	maxPayloadFileBytes     = 64 << 20
	maxPayloadExpandedBytes = 128 << 20
)

// PayloadFile is one regular file from a validated module payload archive.
// Data remains in memory at this stage; validation does not write files or
// execute module code.
type PayloadFile struct {
	Path string
	Mode int64
	Data []byte
}

// ValidatedPayload is the non-executable result of inspecting a verified
// package payload. Package identity is retained so a later filesystem stage
// can reject a payload that does not belong to the package being installed.
// Files are ordered exactly as they appeared in the archive.
type ValidatedPayload struct {
	ModuleID      string
	Version       string
	PayloadSHA256 string
	Files         []PayloadFile

	validationFingerprint [sha256.Size]byte
}

// ValidatePayload parses the opaque payload.tar.gz bytes from a package that
// has already passed VerifyPackage. It enforces bounded expansion and accepts
// only safe directories and regular files. It does not materialize files to
// disk or invoke module code.
func ValidatePayload(pkg VerifiedPackage) (ValidatedPayload, error) {
	if len(pkg.Payload) == 0 {
		return ValidatedPayload{}, errors.New("module payload is empty")
	}

	gz, err := gzip.NewReader(bytes.NewReader(pkg.Payload))
	if err != nil {
		return ValidatedPayload{}, fmt.Errorf("open module payload gzip: %w", err)
	}

	payload, validateErr := validateTarPayload(tar.NewReader(gz))
	closeErr := gz.Close()
	if validateErr != nil {
		return ValidatedPayload{}, validateErr
	}
	if closeErr != nil {
		return ValidatedPayload{}, fmt.Errorf("close module payload gzip: %w", closeErr)
	}
	payload.ModuleID = pkg.Manifest.ID
	payload.Version = pkg.Manifest.Version
	payload.PayloadSHA256 = pkg.PayloadSHA256
	payload.validationFingerprint = fingerprintValidatedPayload(payload.ModuleID, payload.Version, payload.PayloadSHA256, payload.Files)
	return payload, nil
}

func validateTarPayload(reader *tar.Reader) (ValidatedPayload, error) {
	files := make([]PayloadFile, 0)
	seen := make(map[string]struct{})
	var expandedBytes int64

	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ValidatedPayload{}, fmt.Errorf("read module payload header: %w", err)
		}

		clean, err := cleanPayloadPath(header.Name)
		if err != nil {
			return ValidatedPayload{}, err
		}
		if _, duplicate := seen[clean]; duplicate {
			return ValidatedPayload{}, fmt.Errorf("duplicate module payload path %q", clean)
		}
		seen[clean] = struct{}{}

		switch header.Typeflag {
		case tar.TypeDir:
			if header.Size != 0 {
				return ValidatedPayload{}, fmt.Errorf("module payload directory %q has non-zero size", clean)
			}
			continue
		case tar.TypeReg:
			// accepted below
		default:
			return ValidatedPayload{}, fmt.Errorf("module payload path %q uses unsupported tar type %d", clean, header.Typeflag)
		}

		if header.Size < 0 {
			return ValidatedPayload{}, fmt.Errorf("module payload file %q has negative size", clean)
		}
		if header.Mode < 0 || header.Mode&^int64(0o777) != 0 {
			return ValidatedPayload{}, fmt.Errorf("module payload file %q has unsupported mode %#o", clean, header.Mode)
		}
		if err := validatePayloadLimits(len(files), expandedBytes, header.Size); err != nil {
			return ValidatedPayload{}, fmt.Errorf("module payload file %q: %w", clean, err)
		}

		data, err := readBoundedTarFile(reader, header.Size)
		if err != nil {
			return ValidatedPayload{}, fmt.Errorf("read module payload file %q: %w", clean, err)
		}
		expandedBytes += int64(len(data))
		files = append(files, PayloadFile{Path: clean, Mode: header.Mode, Data: data})
	}

	if len(files) == 0 {
		return ValidatedPayload{}, errors.New("module payload contains no regular files")
	}
	return ValidatedPayload{Files: files}, nil
}

func fingerprintValidatedPayload(moduleID, version, payloadSHA256 string, files []PayloadFile) [sha256.Size]byte {
	hash := sha256.New()
	writeBytes := func(data []byte) {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(data)))
		_, _ = hash.Write(length[:])
		_, _ = hash.Write(data)
	}
	writeBytes([]byte(moduleID))
	writeBytes([]byte(version))
	writeBytes([]byte(strings.ToLower(payloadSHA256)))
	var count [8]byte
	binary.BigEndian.PutUint64(count[:], uint64(len(files)))
	_, _ = hash.Write(count[:])
	for _, file := range files {
		writeBytes([]byte(file.Path))
		writeBytes([]byte(strconv.FormatInt(file.Mode, 10)))
		writeBytes(file.Data)
	}
	var fingerprint [sha256.Size]byte
	copy(fingerprint[:], hash.Sum(nil))
	return fingerprint
}

func validatePayloadLimits(fileCount int, expandedBytes, fileSize int64) error {
	if fileCount >= maxPayloadFiles {
		return fmt.Errorf("exceeds %d regular files", maxPayloadFiles)
	}
	if fileSize > maxPayloadFileBytes {
		return fmt.Errorf("exceeds %d bytes", maxPayloadFileBytes)
	}
	if expandedBytes < 0 || expandedBytes > maxPayloadExpandedBytes {
		return errors.New("invalid expanded-byte accounting")
	}
	if fileSize > maxPayloadExpandedBytes-expandedBytes {
		return fmt.Errorf("exceeds %d expanded bytes", maxPayloadExpandedBytes)
	}
	return nil
}

func cleanPayloadPath(name string) (string, error) {
	if name == "" || strings.Contains(name, "\\") {
		return "", fmt.Errorf("unsafe module payload path %q", name)
	}
	clean := path.Clean(name)
	if clean == "." || clean != name || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("unsafe module payload path %q", name)
	}
	return clean, nil
}

func readBoundedTarFile(reader io.Reader, size int64) ([]byte, error) {
	limited := io.LimitReader(reader, size)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != size {
		return nil, io.ErrUnexpectedEOF
	}
	return data, nil
}
