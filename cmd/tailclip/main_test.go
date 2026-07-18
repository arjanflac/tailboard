package main

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestArchiveDirectoryPreservesRelativePathsDeterministically(t *testing.T) {
	source := filepath.Join(t.TempDir(), "folder")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "nested", "hello.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	first, cleanupFirst, err := archiveDirectory(source)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupFirst()
	second, cleanupSecond, err := archiveDirectory(source)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupSecond()
	firstHash, _ := hashFile(first)
	secondHash, _ := hashFile(second)
	if firstHash != secondHash {
		t.Fatalf("archives differ: %s != %s", firstHash, secondHash)
	}

	file, err := os.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader := tar.NewReader(file)
	var names []string
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
	}
	want := []string{"folder/nested", "folder/nested/hello.txt"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("entries = %#v, want %#v", names, want)
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[int64]string{
		500:     "500 B",
		1024:    "1.0 KiB",
		5 << 20: "5.0 MiB",
		3 << 30: "3.0 GiB",
	}
	for value, want := range tests {
		if got := humanBytes(value); got != want {
			t.Fatalf("humanBytes(%d) = %q, want %q", value, got, want)
		}
	}
}
