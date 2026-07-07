package metadata

import (
	"fmt"
	"strconv"
	"strings"

	"ytdl-pro/internal/ytdlpro/tagging"
)

type Candidate struct {
	CandidateID            string            `json:"candidate_id"`
	Source                 string            `json:"source"`
	SourceURL              string            `json:"source_url"`
	SourceTrust            float64           `json:"source_trust"`
	Title                  string            `json:"title"`
	Artist                 string            `json:"artist"`
	Album                  string            `json:"album"`
	AlbumArtist            string            `json:"album_artist"`
	TrackNumber            int               `json:"track_number"`
	DiscNumber             int               `json:"disc_number"`
	Date                   string            `json:"date"`
	Year                   int               `json:"year"`
	DurationSeconds        int               `json:"duration_seconds"`
	Genre                  string            `json:"genre"`
	Label                  string            `json:"label"`
	MusicBrainzRecordingID string            `json:"musicbrainz_recording_id"`
	MusicBrainzReleaseID   string            `json:"musicbrainz_release_id"`
	PreScore               float64           `json:"pre_score"`
	Evidence               EvidenceBreakdown `json:"evidence"`
}

type EvidenceBreakdown struct {
	TitleMatch       float64 `json:"title_match"`
	ArtistMatch      float64 `json:"artist_match"`
	DurationMatch    float64 `json:"duration_match"`
	AlbumMatch       float64 `json:"album_match"`
	TrackNumberMatch float64 `json:"track_number_match"`
}

type FieldDecision struct {
	Value             *string `json:"value"`
	Confidence        float64 `json:"confidence"`
	SourceCandidateID *string `json:"source_candidate_id"`
}

type Decision struct {
	Action               Action                   `json:"action"`
	OverallConfidence    float64                  `json:"overall_confidence"`
	SelectedCandidateIDs []string                 `json:"selected_candidate_ids"`
	ReasonCodes          []string                 `json:"reason_codes"`
	Fields               map[string]FieldDecision `json:"fields"`
	Warnings             []string                 `json:"warnings"`
}

type ItemReport struct {
	Path                 string      `json:"path"`
	VideoID              string      `json:"video_id"`
	Classification       string      `json:"classification"`
	Action               string      `json:"action"`
	OverallConfidence    float64     `json:"overall_confidence"`
	ChangedFields        []string    `json:"changed_fields"`
	SkippedFields        []string    `json:"skipped_fields"`
	SelectedCandidateIDs []string    `json:"selected_candidate_ids"`
	Warnings             []string    `json:"warnings"`
	Diagnostics          Diagnostics `json:"diagnostics"`
	Error                string      `json:"error,omitempty"`
}

type JSONReport struct {
	Summary Summary      `json:"summary"`
	Items   []ItemReport `json:"items"`
}

type Summary struct {
	FilesScanned    int `json:"files_scanned"`
	FilesChanged    int `json:"files_changed"`
	FilesBaseTagged int `json:"files_base_tagged"`
	FilesSkipped    int `json:"files_skipped"`
	NeedsReview     int `json:"needs_review"`
	Errors          int `json:"errors"`
	ModelAttempted  int `json:"model_attempted"`
	ModelAccepted   int `json:"model_accepted"`
	ModelFallbacks  int `json:"model_fallbacks"`
	ModelUnused     int `json:"model_unused"`
}

type Diagnostics struct {
	CandidateCount          int              `json:"candidate_count"`
	TopCandidateIDs         []string         `json:"top_candidate_ids,omitempty"`
	DeterministicAction     string           `json:"deterministic_action,omitempty"`
	DeterministicConfidence float64          `json:"deterministic_confidence,omitempty"`
	FinalDecisionSource     string           `json:"final_decision_source,omitempty"`
	Model                   ModelDiagnostics `json:"model"`
}

type ModelDiagnostics struct {
	Backend                 string   `json:"backend,omitempty"`
	Model                   string   `json:"model,omitempty"`
	Attempted               bool     `json:"attempted"`
	Accepted                bool     `json:"accepted"`
	FallbackReason          string   `json:"fallback_reason,omitempty"`
	Error                   string   `json:"error,omitempty"`
	RawAction               string   `json:"raw_action,omitempty"`
	RawOverallConfidence    float64  `json:"raw_overall_confidence,omitempty"`
	RawSelectedCandidateIDs []string `json:"raw_selected_candidate_ids,omitempty"`
}

func (c Candidate) ToTags() tagging.Tags {
	tags := tagging.Tags{
		Title:                  strings.TrimSpace(c.Title),
		Artist:                 strings.TrimSpace(c.Artist),
		Album:                  strings.TrimSpace(c.Album),
		AlbumArtist:            strings.TrimSpace(c.AlbumArtist),
		Date:                   strings.TrimSpace(c.Date),
		Genre:                  strings.TrimSpace(c.Genre),
		Label:                  strings.TrimSpace(c.Label),
		MusicBrainzRecordingID: strings.TrimSpace(c.MusicBrainzRecordingID),
		MusicBrainzReleaseID:   strings.TrimSpace(c.MusicBrainzReleaseID),
	}
	if c.Year > 0 {
		tags.Year = strconv.Itoa(c.Year)
	}
	if c.TrackNumber > 0 {
		tags.TrackNumber = strconv.Itoa(c.TrackNumber)
	}
	if c.DiscNumber > 0 {
		tags.DiscNumber = strconv.Itoa(c.DiscNumber)
	}
	return tags
}

func ExistingTagsCandidate(tags tagging.Tags) *Candidate {
	values := tags.Values()
	if len(values) == 0 {
		return nil
	}

	sourceTrust := 0.70
	if tags.MusicBrainzRecordingID != "" || tags.MusicBrainzReleaseID != "" {
		sourceTrust = 1.0
	}

	year := 0
	if len(tags.Date) >= 4 {
		if parsedYear, err := strconv.Atoi(tags.Date[:4]); err == nil {
			year = parsedYear
		}
	}

	return &Candidate{
		CandidateID:            fmt.Sprintf("existing:%s:%s", safeCandidateToken(tags.Title), safeCandidateToken(tags.Artist)),
		Source:                 "existing_tags",
		SourceTrust:            sourceTrust,
		Title:                  tags.Title,
		Artist:                 tags.Artist,
		Album:                  tags.Album,
		AlbumArtist:            tags.AlbumArtist,
		Date:                   tags.Date,
		Year:                   year,
		Genre:                  tags.Genre,
		Label:                  tags.Label,
		MusicBrainzRecordingID: tags.MusicBrainzRecordingID,
		MusicBrainzReleaseID:   tags.MusicBrainzReleaseID,
	}
}

func safeCandidateToken(value string) string {
	value = normalizeForComparison(value)
	if value == "" {
		return "unknown"
	}
	return strings.ReplaceAll(value, " ", "_")
}
