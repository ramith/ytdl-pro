package ytdlpro

import "testing"

func TestIsPlaylistURL(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.youtube.com/playlist?list=PL123", true},
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

func TestPlaylistRejectsExplicitFilename(t *testing.T) {
	_, err := ParseConfig([]string{
		"-url", "PL123",
		"-playlist",
		"-filename", "same.mp3",
		"-i-have-rights",
	})
	if err == nil {
		t.Fatal("expected playlist filename validation error")
	}
}
