package modules

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestReadZipEntryEnforcesStreamingLimit(t *testing.T) {
	file := testZipFile(t, []byte("abc"))

	data, err := readZipEntry(file, 3)
	if err != nil {
		t.Fatalf("readZipEntry exact limit returned error: %v", err)
	}
	if string(data) != "abc" {
		t.Fatalf("readZipEntry data = %q, want %q", data, "abc")
	}

	file = testZipFile(t, []byte("abc"))
	if _, err := readZipEntry(file, 2); err == nil {
		t.Fatal("readZipEntry succeeded above limit, want rejection")
	}
}

func TestReadZipEntryRejectsNegativeLimit(t *testing.T) {
	file := testZipFile(t, []byte("a"))
	if _, err := readZipEntry(file, -1); err == nil {
		t.Fatal("readZipEntry succeeded with negative limit, want rejection")
	}
}

func testZipFile(t *testing.T, contents []byte) *zip.File {
	t.Helper()

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("entry")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(contents); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	reader, err := zip.NewReader(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) != 1 {
		t.Fatalf("zip file count = %d, want 1", len(reader.File))
	}
	return reader.File[0]
}
