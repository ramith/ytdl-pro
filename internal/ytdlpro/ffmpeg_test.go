package ytdlpro

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestFFmpegMuxerForPath(t *testing.T) {
	tests := map[string]string{
		"audio.mp3":  "mp3",
		"audio.flac": "flac",
		"audio.wav":  "wav",
		"audio.m4a":  "ipod",
		"video.mp4":  "mp4",
		"video.mkv":  "matroska",
	}

	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			got, err := FFmpegMuxerForPath(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != want {
				t.Fatalf("muxer = %q, want %q", got, want)
			}
		})
	}
}

func TestFFmpegMuxerForPathRejectsUnknownExtension(t *testing.T) {
	if _, err := FFmpegMuxerForPath("output.part"); err == nil {
		t.Fatal("expected unknown extension error")
	}
}

func TestTranscodeAudioWritesMP3ThroughPartFile(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}

	dir := t.TempDir()
	source := filepath.Join(dir, "source.webm")
	output := filepath.Join(dir, "output.mp3")

	cmd := exec.Command(
		"ffmpeg",
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=1000:duration=0.1",
		"-codec:a", "libopus",
		source,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create WebM source: %v: %s", err, output)
	}

	cfg := Config{
		AudioFormat: AudioMP3,
		MP3Mode:     MP3VBR,
		MP3VBR:      0,
	}
	if err := (FFmpeg{}).TranscodeAudio(context.Background(), source, output, cfg); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("MP3 output is empty")
	}
}
