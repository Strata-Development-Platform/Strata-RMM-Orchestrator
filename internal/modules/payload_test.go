package modules

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestValidatePayloadAcceptsSafeRegularFiles(t *testing.T) {
	pkg := VerifiedPackage{Payload: makePayloadArchive(t, []payloadArchiveEntry{
		{name: "bin", typeflag: tar.TypeDir, mode: 0o755},
		{name: "bin/module", typeflag: tar.TypeReg, mode: 0o755, data: []byte("binary")},
		{name: "config/default.json", typeflag: tar.TypeReg, mode: 0o644, data: []byte(`{"enabled":true}`)},
	})}

	validated, err := ValidatePayload(pkg)
	if err != nil {
		t.Fatalf("ValidatePayload returned error: %v", err)
	}
	if len(validated.Files) != 2 {
		t.Fatalf("files = %d, want 2", len(validated.Files))
	}
	if validated.Files[0].Path != "bin/module" || validated.Files[0].Mode != 0o755 || string(validated.Files[0].Data) != "binary" {
		t.Fatalf("first file = %#v", validated.Files[0])
	}
	if validated.Files[1].Path != "config/default.json" || validated.Files[1].Mode != 0o644 {
		t.Fatalf("second file = %#v", validated.Files[1])
	}
}

func TestValidatePayloadRejectsUnsafePaths(t *testing.T) {
	tests := []string{"../escape", "/absolute", "nested/../escape", `windows\\path`}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			pkg := VerifiedPackage{Payload: makePayloadArchive(t, []payloadArchiveEntry{{name: name, typeflag: tar.TypeReg, mode: 0o644, data: []byte("x")}})}
			if _, err := ValidatePayload(pkg); err == nil {
				t.Fatal("ValidatePayload succeeded, want unsafe path rejection")
			}
		})
	}
}

func TestValidatePayloadRejectsLinksAndSpecialFiles(t *testing.T) {
	tests := []byte{tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo}
	for _, typeflag := range tests {
		t.Run(string([]byte{typeflag}), func(t *testing.T) {
			pkg := VerifiedPackage{Payload: makePayloadArchive(t, []payloadArchiveEntry{{name: "entry", typeflag: typeflag, mode: 0o644}})}
			if _, err := ValidatePayload(pkg); err == nil {
				t.Fatal("ValidatePayload succeeded, want unsupported type rejection")
			}
		})
	}
}

func TestValidatePayloadRejectsDuplicatePaths(t *testing.T) {
	pkg := VerifiedPackage{Payload: makePayloadArchive(t, []payloadArchiveEntry{
		{name: "module", typeflag: tar.TypeReg, mode: 0o755, data: []byte("one")},
		{name: "module", typeflag: tar.TypeReg, mode: 0o755, data: []byte("two")},
	})}
	if _, err := ValidatePayload(pkg); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("ValidatePayload error = %v, want duplicate rejection", err)
	}
}

func TestValidatePayloadRejectsUnsupportedMode(t *testing.T) {
	pkg := VerifiedPackage{Payload: makePayloadArchive(t, []payloadArchiveEntry{{name: "module", typeflag: tar.TypeReg, mode: 0o4755, data: []byte("x")}})}
	if _, err := ValidatePayload(pkg); err == nil || !strings.Contains(err.Error(), "unsupported mode") {
		t.Fatalf("ValidatePayload error = %v, want mode rejection", err)
	}
}

func TestValidatePayloadRejectsEmptyAndInvalidGzip(t *testing.T) {
	if _, err := ValidatePayload(VerifiedPackage{}); err == nil {
		t.Fatal("ValidatePayload accepted empty payload")
	}
	if _, err := ValidatePayload(VerifiedPackage{Payload: []byte("not-gzip")}); err == nil {
		t.Fatal("ValidatePayload accepted invalid gzip")
	}
}

func TestValidatePayloadRejectsArchiveWithoutRegularFiles(t *testing.T) {
	pkg := VerifiedPackage{Payload: makePayloadArchive(t, []payloadArchiveEntry{{name: "empty", typeflag: tar.TypeDir, mode: 0o755}})}
	if _, err := ValidatePayload(pkg); err == nil {
		t.Fatal("ValidatePayload accepted archive without regular files")
	}
}

func TestValidatePayloadRejectsOversizedFileFromHeader(t *testing.T) {
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "huge", Mode: 0o644, Size: maxPayloadFileBytes + 1, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	// The header is enough to trigger validation before the missing body matters.
	_ = tw.Close()
	_ = gz.Close()

	_, err := ValidatePayload(VerifiedPackage{Payload: buffer.Bytes()})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("ValidatePayload error = %v, want file-size rejection", err)
	}
}

func TestReadBoundedTarFileRequiresDeclaredSize(t *testing.T) {
	_, err := readBoundedTarFile(bytes.NewReader([]byte("x")), 2)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("readBoundedTarFile error = %v, want io.ErrUnexpectedEOF", err)
	}
}

type payloadArchiveEntry struct {
	name     string
	typeflag byte
	mode     int64
	data     []byte
}

func makePayloadArchive(t *testing.T, entries []payloadArchiveEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gz := gzip.NewWriter(&buffer)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		size := int64(len(entry.data))
		if entry.typeflag == tar.TypeDir {
			size = 0
		}
		header := &tar.Header{Name: entry.name, Mode: entry.mode, Size: size, Typeflag: entry.typeflag}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if size > 0 {
			if _, err := tw.Write(entry.data); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
