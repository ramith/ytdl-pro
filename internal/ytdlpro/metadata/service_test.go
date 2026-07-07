package metadata

import (
	"context"
	"strings"
	"testing"
	"time"

	"ytdl-pro/internal/ytdlpro/tagging"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		title    string
		duration time.Duration
		want     Classification
	}{
		{"Artist - Song Title (Official Video)", 4 * time.Minute, ClassMusicVideo},
		{"Artist - Song Title Lyrics", 4 * time.Minute, ClassLyricsVideo},
		{"Best Sinhala Songs 2024 Nonstop Mix", 45 * time.Minute, ClassLongMix},
		{"Weekly Podcast Episode 12", 52 * time.Minute, ClassPodcast},
	}

	for _, test := range tests {
		t.Run(test.title, func(t *testing.T) {
			if got := Classify(test.title, test.duration); got != test.want {
				t.Fatalf("Classify() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestDeterministicDecisionFallsBackToBaseTags(t *testing.T) {
	base := BuildBaseMetadata(Request{
		Title:       "Artist - Song",
		Channel:     "Artist",
		URL:         "https://www.youtube.com/watch?v=abc123",
		VideoID:     "abc123",
		Duration:    4 * time.Minute,
		UploadDate:  time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC),
		Description: "demo",
	})

	decision := deterministicDecision(base, nil, Options{WriteBaseTags: true, MinFullConfidence: 0.90, MinFieldConfidence: 0.85, MinReviewConfidence: 0.70})
	if decision.Action != ActionWriteBase {
		t.Fatalf("decision.Action = %q, want %q", decision.Action, ActionWriteBase)
	}
}

func TestBuildWritePlanKeepsOnlyHighConfidenceFields(t *testing.T) {
	base := BuildBaseMetadata(Request{
		Title:   "Artist - Song",
		Channel: "Artist",
		URL:     "https://www.youtube.com/watch?v=abc123",
		VideoID: "abc123",
	})
	title := "Song"
	artist := "Artist"
	album := "Album"

	planned, changed, skipped := buildWritePlan(base, tagging.Tags{}, Decision{
		Action: ActionWritePartial,
		Fields: map[string]FieldDecision{
			"title":  {Value: &title, Confidence: 0.95, SourceCandidateID: stringPtr("candidate-1")},
			"artist": {Value: &artist, Confidence: 0.94, SourceCandidateID: stringPtr("candidate-1")},
			"album":  {Value: &album, Confidence: 0.40, SourceCandidateID: stringPtr("candidate-1")},
		},
	}, Options{MinFieldConfidence: 0.85})

	if planned.Title != "Song" || planned.Artist != "Artist" {
		t.Fatalf("planned tags missing expected fields: %+v", planned)
	}
	if planned.Album != "" {
		t.Fatalf("planned.Album = %q, want empty", planned.Album)
	}
	if len(changed) != 3 {
		t.Fatalf("changed field count = %d, want 3 (title, artist, comment)", len(changed))
	}
	if len(skipped) == 0 || skipped[0] != "album" {
		t.Fatalf("skipped = %#v, want album to be skipped", skipped)
	}
}

func TestNewServiceWithoutLibllamaFallsBackGracefully(t *testing.T) {
	service := NewService(Options{
		Runtime:             "libllama",
		Model:               "qwen3-1.7b-instruct-q4_k_m",
		ModelPath:           "./models/qwen3-1.7b-instruct-q4_k_m.gguf",
		GrammarPath:         "./grammars/metadata-decision.gbnf",
		ContextTokens:       4096,
		MaxOutputTokens:     512,
		MinFullConfidence:   0.90,
		MinFieldConfidence:  0.85,
		MinReviewConfidence: 0.70,
	})

	_, _, diagnostics, _ := service.resolve(context.Background(), BuildBaseMetadata(Request{
		Title:   "Artist - Song",
		Channel: "Artist",
	}), tagging.Tags{})

	if diagnostics.Model.FallbackReason == "" {
		t.Fatal("expected runtime fallback diagnostics")
	}
}

func TestDecodeAndValidateDecisionRejectsUnknownFields(t *testing.T) {
	candidates := []Candidate{{CandidateID: "musicbrainz:recording:test"}}
	raw := strings.TrimSuffix(validDecisionJSON(), "\n}") + ",\n  \"unexpected\": \"value\"\n}"

	_, err := decodeAndValidateDecision(raw, candidates)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("expected unknown field error, got %v", err)
	}
}

func TestDecodeAndValidateDecisionRejectsMissingSourceCandidateID(t *testing.T) {
	candidates := []Candidate{{CandidateID: "musicbrainz:recording:test"}}
	raw := `{
  "action": "write_partial",
  "overall_confidence": 0.88,
  "selected_candidate_ids": ["musicbrainz:recording:test"],
  "reason_codes": ["title_artist_duration_match"],
  "fields": {
    "title": {"value": "Song", "confidence": 0.96, "source_candidate_id": null},
    "artist": {"value": "Artist", "confidence": 0.95, "source_candidate_id": "musicbrainz:recording:test"},
    "album": {"value": null, "confidence": 0.48, "source_candidate_id": null},
    "album_artist": {"value": null, "confidence": 0.40, "source_candidate_id": null},
    "date": {"value": null, "confidence": 0.35, "source_candidate_id": null},
    "genre": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "label": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "track_number": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "disc_number": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "musicbrainz_recording_id": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "musicbrainz_release_id": {"value": null, "confidence": 0.20, "source_candidate_id": null}
  },
  "warnings": []
}`

	_, err := decodeAndValidateDecision(raw, candidates)
	if err == nil || !strings.Contains(err.Error(), "missing source_candidate_id") {
		t.Fatalf("expected missing source candidate error, got %v", err)
	}
}

func validDecisionJSON() string {
	return `{
  "action": "write_partial",
  "overall_confidence": 0.88,
  "selected_candidate_ids": ["musicbrainz:recording:test"],
  "reason_codes": ["title_artist_duration_match"],
  "fields": {
    "title": {"value": "Song", "confidence": 0.96, "source_candidate_id": "musicbrainz:recording:test"},
    "artist": {"value": "Artist", "confidence": 0.95, "source_candidate_id": "musicbrainz:recording:test"},
    "album": {"value": null, "confidence": 0.48, "source_candidate_id": null},
    "album_artist": {"value": null, "confidence": 0.40, "source_candidate_id": null},
    "date": {"value": null, "confidence": 0.35, "source_candidate_id": null},
    "genre": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "label": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "track_number": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "disc_number": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "musicbrainz_recording_id": {"value": null, "confidence": 0.20, "source_candidate_id": null},
    "musicbrainz_release_id": {"value": null, "confidence": 0.20, "source_candidate_id": null}
  },
  "warnings": []
}`
}

func stringPtr(value string) *string {
	return &value
}
