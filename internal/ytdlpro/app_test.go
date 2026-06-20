package ytdlpro

import "testing"

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

func TestParseConfigNoRightsFlagRequired(t *testing.T) {
	_, err := ParseConfig([]string{
		"-url", "https://www.youtube.com/watch?v=abc123",
	})
	if err != nil {
		t.Fatalf("expected config to validate without -i-have-rights, got %v", err)
	}
}

func TestParseConfigSmartAudioAllowsMP3Options(t *testing.T) {
	cfg, err := ParseConfig([]string{
		"-url", "https://www.youtube.com/watch?v=abc123",
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
