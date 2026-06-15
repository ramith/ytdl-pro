package ytdlpro

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseConfigPositionalURLStartsInteractiveMode(t *testing.T) {
	cfg, err := ParseConfig([]string{"https://youtu.be/example"})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.URL != "https://youtu.be/example" {
		t.Fatalf("URL = %q", cfg.URL)
	}
	if !cfg.Interactive {
		t.Fatal("expected interactive mode")
	}
}

func TestParseConfigFlagsRemainNonInteractive(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"-url", "https://youtu.be/example",
		"-i-have-rights",
		"-audio-only",
	})
	if err != nil {
		t.Fatal(err)
	}

	if cfg.Interactive {
		t.Fatal("expected non-interactive mode")
	}
}

func TestCompleteInteractiveVideoDefaults(t *testing.T) {
	cfg, err := ParseConfig([]string{"https://youtu.be/example"})
	if err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	cfg, err = CompleteInteractive(strings.NewReader("y\n\n\n\n"), &output, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.RightsOK || cfg.AudioOnly || cfg.VideoQuality != "best" || cfg.OutDir != "." || cfg.Overwrite {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestCompleteInteractiveMP3(t *testing.T) {
	cfg, err := ParseConfig([]string{"https://youtu.be/example"})
	if err != nil {
		t.Fatal(err)
	}

	input := "yes\naudio\nhigh\nmp3\nbitrate\n320k\n./downloads\n"
	var output bytes.Buffer
	cfg, err = CompleteInteractive(strings.NewReader(input), &output, cfg)
	if err != nil {
		t.Fatal(err)
	}

	if !cfg.AudioOnly || cfg.AudioQuality != "high" || cfg.AudioFormat != AudioMP3 {
		t.Fatalf("unexpected audio config: %+v", cfg)
	}
	if cfg.MP3Mode != MP3Bitrate || cfg.MP3Bitrate != "320k" {
		t.Fatalf("unexpected MP3 config: %+v", cfg)
	}
	if cfg.OutDir != "./downloads" || cfg.Filename != "" || cfg.Overwrite {
		t.Fatalf("unexpected output config: %+v", cfg)
	}
}
