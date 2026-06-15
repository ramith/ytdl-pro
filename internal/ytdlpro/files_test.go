package ytdlpro

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveOutputPathUsesTitleAndAddsSuffix(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"Example Video.mp4", "Example Video (1).mp4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("existing"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	path, err := ResolveOutputPath(dir, "", "Example Video", ".mp4", false)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, "Example Video (2).mp4")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestResolveOutputPathKeepsExplicitFilename(t *testing.T) {
	dir := t.TempDir()

	path, err := ResolveOutputPath(dir, "custom", "Example Video", ".mp4", false)
	if err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, "custom.mp4")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}

func TestResolveOutputPathKeepsTitleWhenOverwriting(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "Example Video.mp4")
	if err := os.WriteFile(existing, []byte("existing"), 0644); err != nil {
		t.Fatal(err)
	}

	path, err := ResolveOutputPath(dir, "", "Example Video", ".mp4", true)
	if err != nil {
		t.Fatal(err)
	}

	if path != existing {
		t.Fatalf("path = %q, want %q", path, existing)
	}
}
