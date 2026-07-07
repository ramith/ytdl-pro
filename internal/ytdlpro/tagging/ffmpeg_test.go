package tagging

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestWriteTagsRoundTripMP3(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe is not installed")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source.mp3")

	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=0.2",
		"-codec:a", "libmp3lame",
		source,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create MP3 source: %v: %s", err, output)
	}

	tags := Tags{
		Title:   "Song Title",
		Artist:  "Artist Name",
		Album:   "Album Name",
		Comment: "source_url=https://www.youtube.com/watch?v=abc123 video_id=abc123",
	}
	if err := WriteTags(context.Background(), source, tags, WriteOptions{KeepBackup: true}); err != nil {
		t.Fatal(err)
	}

	probe, err := ReadTags(context.Background(), source)
	if err != nil {
		t.Fatal(err)
	}
	if probe.Tags.Title != tags.Title {
		t.Fatalf("title = %q, want %q", probe.Tags.Title, tags.Title)
	}
	if probe.Tags.Artist != tags.Artist {
		t.Fatalf("artist = %q, want %q", probe.Tags.Artist, tags.Artist)
	}
	if probe.Tags.Album != tags.Album {
		t.Fatalf("album = %q, want %q", probe.Tags.Album, tags.Album)
	}
	if probe.Tags.Comment != tags.Comment {
		t.Fatalf("comment = %q, want %q", probe.Tags.Comment, tags.Comment)
	}
	if _, err := os.Stat(source + ".bak"); err != nil {
		t.Fatalf("expected backup file %s.bak to exist", source)
	}
}
