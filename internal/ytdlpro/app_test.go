package ytdlpro

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsPlaylistURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/playlist?list=PL123", true},
		{"https://music.youtube.com/playlist?list=PL123", true},
		{"https://www.youtube.com/watch?v=abc&list=PL123", true},
		{"https://youtu.be/abc?list=PL123", true},
		{"PLQZgI7en5XEgM0L1_ZcKmEzxW1sCOVZwP", true},
		{"https://www.youtube.com/watch?v=ueBE7WIeqZU&list=RDueBE7WIeqZU&start_radio=1", false},
		{"RDueBE7WIeqZU", false},
		{"https://www.youtube.com/watch?v=abc", false},
		{"abc", false},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			if got := IsPlaylistURL(test.url); got != test.want {
				t.Fatalf("IsPlaylistURL() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestIsRadioMixID(t *testing.T) {
	for _, id := range []string{"RDueBE7WIeqZU", "RDCLAK5uy_example", "rdamvm_example"} {
		if !IsRadioMixID(id) {
			t.Fatalf("expected %q to be detected as a radio mix", id)
		}
	}
}

func TestNormalizeYouTubeURL(t *testing.T) {
	tests := []struct {
		raw  string
		want string
	}{
		{"https://music.youtube.com/playlist?list=PL_wnztHRappIJTUlaW64gfU1RVmBka8XM", "https://www.youtube.com/playlist?list=PL_wnztHRappIJTUlaW64gfU1RVmBka8XM"},
		{"https://m.youtube.com/watch?v=abc123", "https://www.youtube.com/watch?v=abc123"},
		{"https://www.youtube.com/watch?v=abc123", "https://www.youtube.com/watch?v=abc123"},
		{"PLQZgI7en5XEgM0L1_ZcKmEzxW1sCOVZwP", "PLQZgI7en5XEgM0L1_ZcKmEzxW1sCOVZwP"},
	}

	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			if got := NormalizeYouTubeURL(test.raw); got != test.want {
				t.Fatalf("NormalizeYouTubeURL() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPlaylistRejectsExplicitFilename(t *testing.T) {
	_, err := ParseConfig([]string{
		"-url", "PL123",
		"-playlist",
		"-filename", "same.mp3",
	})
	if err == nil {
		t.Fatal("expected playlist filename validation error")
	}
}

func TestParseConfigSmartAudioAllowsMP3Options(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"download",
		"https://www.youtube.com/watch?v=abc123",
		"-audio-only",
		"-audio-format", "smart",
		"-mp3-mode", "vbr",
		"-mp3-vbr", "0",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.AudioFormat != AudioSmart {
		t.Fatalf("AudioFormat = %q, want %q", cfg.AudioFormat, AudioSmart)
	}
}

func TestParseConfigEnrichDryRun(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"enrich",
		"https://www.youtube.com/watch?v=abc123",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if !cfg.Metadata.Enabled {
		t.Fatal("expected metadata pipeline to be enabled")
	}
	if !cfg.Metadata.DryRun {
		t.Fatal("expected metadata mode to default to dry-run")
	}
	if cfg.Metadata.Write {
		t.Fatal("expected metadata writes to remain disabled by default")
	}
}

func TestParseConfigEnrichDefaultsToWrite(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"enrich",
		"https://www.youtube.com/watch?v=abc123",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if !cfg.Metadata.Enabled {
		t.Fatal("expected metadata pipeline to be enabled")
	}
	if cfg.Metadata.DryRun {
		t.Fatal("expected enrich to default to writable mode")
	}
	if !cfg.Metadata.Write {
		t.Fatal("expected enrich writes to be enabled")
	}
}

func TestDefaultConfigHasSensibleDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.OutDir != "." {
		t.Fatalf("OutDir = %q, want %q", cfg.OutDir, ".")
	}
	if cfg.Timeout <= 0 {
		t.Fatalf("Timeout = %s, want positive default", cfg.Timeout)
	}
	if cfg.VideoQuality != "best" {
		t.Fatalf("VideoQuality = %q, want %q", cfg.VideoQuality, "best")
	}
	if cfg.AudioQuality != "best" {
		t.Fatalf("AudioQuality = %q, want %q", cfg.AudioQuality, "best")
	}
	if cfg.AudioFormat != AudioOriginal {
		t.Fatalf("AudioFormat = %q, want %q", cfg.AudioFormat, AudioOriginal)
	}
	if cfg.MP3Mode != MP3VBR {
		t.Fatalf("MP3Mode = %q, want %q", cfg.MP3Mode, MP3VBR)
	}
	if cfg.MP3Bitrate != "192k" {
		t.Fatalf("MP3Bitrate = %q, want %q", cfg.MP3Bitrate, "192k")
	}
	if cfg.Metadata.Model != "qwen3-1.7b-instruct-q4_k_m" {
		t.Fatalf("Metadata.Model = %q, want %q", cfg.Metadata.Model, "qwen3-1.7b-instruct-q4_k_m")
	}
	if cfg.Metadata.Runtime != "libllama" {
		t.Fatalf("Metadata.Runtime = %q, want %q", cfg.Metadata.Runtime, "libllama")
	}
}

func TestParseConfigUsesDefaultsForNonInteractiveFlags(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"download",
		"https://www.youtube.com/watch?v=abc123",
		"-audio-only",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.AudioFormat != AudioOriginal {
		t.Fatalf("AudioFormat = %q, want %q", cfg.AudioFormat, AudioOriginal)
	}
	if cfg.MP3Mode != MP3VBR {
		t.Fatalf("MP3Mode = %q, want %q", cfg.MP3Mode, MP3VBR)
	}
	if cfg.AudioQuality != "best" {
		t.Fatalf("AudioQuality = %q, want %q", cfg.AudioQuality, "best")
	}
	if cfg.Metadata.Timeout != 2*time.Minute {
		t.Fatalf("Metadata.Timeout = %s, want %s", cfg.Metadata.Timeout, 2*time.Minute)
	}
}

func TestParseConfigAllowsEnrichPathWithoutURL(t *testing.T) {
	dir := t.TempDir()
	track := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(track, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseConfig([]string{
		"enrich",
		track,
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.Metadata.Path != track {
		t.Fatalf("Metadata.Path = %q, want %q", cfg.Metadata.Path, track)
	}
	if cfg.URL != "" {
		t.Fatalf("URL = %q, want empty", cfg.URL)
	}
}

func TestParseConfigDownloadCommandUsesDefaults(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"download",
		"https://www.youtube.com/watch?v=abc123",
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.Command != "download" {
		t.Fatalf("Command = %q, want %q", cfg.Command, "download")
	}
	if cfg.Interactive {
		t.Fatal("expected explicit download command to be non-interactive")
	}
}

func TestParseConfigBareURLStartsInteractiveMode(t *testing.T) {
	cfg, err := ParseConfig([]string{"https://www.youtube.com/watch?v=abc123"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.Command != "download" {
		t.Fatalf("Command = %q, want %q", cfg.Command, "download")
	}
	if !cfg.Interactive {
		t.Fatal("expected bare URL shortcut to remain interactive")
	}
}

func TestParseConfigEnrichPositionalPath(t *testing.T) {
	dir := t.TempDir()
	track := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(track, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseConfig([]string{
		"enrich",
		track,
	})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.Metadata.Path != track {
		t.Fatalf("Metadata.Path = %q, want %q", cfg.Metadata.Path, track)
	}
	if cfg.URL != "" {
		t.Fatalf("URL = %q, want empty", cfg.URL)
	}
}

func TestParseConfigRejectsURLWithEnrichPath(t *testing.T) {
	dir := t.TempDir()
	track := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(track, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	_, err := ParseConfig([]string{
		"enrich",
		"-url", "https://www.youtube.com/watch?v=abc123",
		track,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestParseConfigEnrichCommandDefaults(t *testing.T) {
	dir := t.TempDir()
	track := filepath.Join(dir, "track.mp3")
	if err := os.WriteFile(track, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := ParseConfig([]string{"enrich", track})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.Command != "enrich" {
		t.Fatalf("Command = %q, want %q", cfg.Command, "enrich")
	}
	if !cfg.AudioOnly {
		t.Fatal("expected enrich to default to audio-only mode")
	}
	if !cfg.Metadata.Enabled || !cfg.Metadata.Write {
		t.Fatal("expected enrich to enable writable metadata processing")
	}
	if cfg.Metadata.Path != track {
		t.Fatalf("Metadata.Path = %q, want %q", cfg.Metadata.Path, track)
	}
}

func TestParseConfigSetupCommandDefaults(t *testing.T) {
	cfg, err := ParseConfig([]string{"setup"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	if cfg.Command != "setup" {
		t.Fatalf("Command = %q, want %q", cfg.Command, "setup")
	}
	if cfg.Setup.ModelPath != "./models/qwen3-1.7b-instruct-q4_k_m.gguf" {
		t.Fatalf("Setup.ModelPath = %q", cfg.Setup.ModelPath)
	}
	if cfg.Metadata.ModelPath != cfg.Setup.ModelPath {
		t.Fatalf("Metadata.ModelPath = %q, want %q", cfg.Metadata.ModelPath, cfg.Setup.ModelPath)
	}
}

func TestParseConfigSetupRejectsPositionals(t *testing.T) {
	_, err := ParseConfig([]string{"setup", "extra"})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
