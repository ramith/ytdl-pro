package ytdlpro

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestCollectMetadataPathsSingleFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "song.mp3")
	if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	paths, err := collectMetadataPaths(path, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !reflect.DeepEqual(paths, []string{path}) {
		t.Fatalf("paths = %#v, want %#v", paths, []string{path})
	}
}

func TestCollectMetadataPathsDirectoryNonRecursive(t *testing.T) {
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "a.mp3")
	flac := filepath.Join(dir, "b.flac")
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	subMP3 := filepath.Join(sub, "c.mp3")

	for _, path := range []string{mp3, flac, subMP3, filepath.Join(dir, "ignore.txt")} {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := collectMetadataPaths(dir, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{mp3, flac}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}

func TestCollectMetadataPathsDirectoryRecursive(t *testing.T) {
	dir := t.TempDir()
	mp3 := filepath.Join(dir, "a.mp3")
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	subMP3 := filepath.Join(sub, "c.mp3")

	for _, path := range []string{mp3, subMP3} {
		if err := os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := collectMetadataPaths(dir, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{mp3, subMP3}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %#v, want %#v", paths, want)
	}
}
