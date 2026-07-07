package metadata

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"ytdl-pro/internal/ytdlpro/tagging"
)

type Classification string

const (
	ClassOfficialTrack   Classification = "official_track"
	ClassMusicVideo      Classification = "music_video"
	ClassLyricsVideo     Classification = "lyrics_video"
	ClassLivePerformance Classification = "live_performance"
	ClassCover           Classification = "cover"
	ClassRemix           Classification = "remix"
	ClassPodcast         Classification = "podcast"
	ClassSpeech          Classification = "speech"
	ClassPlaylistMix     Classification = "playlist_mix"
	ClassLongMix         Classification = "long_mix"
	ClassUnknown         Classification = "unknown"
)

type Action string

const (
	ActionWriteFull    Action = "write_full"
	ActionWritePartial Action = "write_partial"
	ActionWriteBase    Action = "write_base_only"
	ActionSkip         Action = "skip"
	ActionNeedsReview  Action = "needs_review"
)

type Request struct {
	Path          string
	VideoID       string
	URL           string
	Title         string
	Channel       string
	Description   string
	PlaylistTitle string
	Duration      time.Duration
	UploadDate    time.Time
}

type Options struct {
	DryRun              bool
	Write               bool
	ReviewOnly          bool
	Explain             bool
	Debug               bool
	WriteBaseTags       bool
	Model               string
	Runtime             string
	ModelPath           string
	GrammarPath         string
	ContextTokens       int
	MaxOutputTokens     int
	Threads             string
	GPULayers           string
	MinFullConfidence   float64
	MinFieldConfidence  float64
	MinReviewConfidence float64
	JSONReport          string
	Timeout             time.Duration
	Retries             int
	KeepBackup          bool
	SourceMusicBrainz   bool
}

type BaseMetadata struct {
	Request
	NormalizedTitle string
	SearchTitle     string
	SearchArtist    string
	Classification  Classification
	BaseTags        tagging.Tags
}

var bracketNoisePattern = regexp.MustCompile(`(?i)\[(official|lyrics?|lyric video|audio|video|hd|4k|hq)[^\]]*\]|\((official|lyrics?|lyric video|audio|video|hd|4k|hq)[^)]*\)`)

func BuildBaseMetadata(req Request) BaseMetadata {
	searchArtist, searchTitle := deriveSearchTerms(req.Title, req.Channel)
	classification := Classify(req.Title, req.Duration)

	baseDate := ""
	if !req.UploadDate.IsZero() {
		baseDate = req.UploadDate.Format("2006-01-02")
	}

	comment := buildSourceComment(req.URL, req.VideoID, req.Path)

	return BaseMetadata{
		Request:         req,
		NormalizedTitle: normalizeForComparison(req.Title),
		SearchTitle:     searchTitle,
		SearchArtist:    searchArtist,
		Classification:  classification,
		BaseTags: tagging.Tags{
			Title:   strings.TrimSpace(req.Title),
			Artist:  strings.TrimSpace(req.Channel),
			Date:    baseDate,
			Comment: strings.TrimSpace(comment),
		},
	}
}

func deriveSearchTerms(title, channel string) (string, string) {
	cleanedTitle := cleanSearchTitle(title)
	for _, delimiter := range []string{" - ", " – ", " — "} {
		parts := strings.SplitN(cleanedTitle, delimiter, 2)
		if len(parts) != 2 {
			continue
		}
		left := strings.TrimSpace(parts[0])
		right := strings.TrimSpace(parts[1])
		if left != "" && right != "" {
			return left, right
		}
	}
	return strings.TrimSpace(channel), cleanedTitle
}

func cleanSearchTitle(title string) string {
	title = stripControlCharacters(title)
	title = bracketNoisePattern.ReplaceAllString(title, "")
	title = strings.TrimSpace(title)
	title = strings.TrimSuffix(title, "- Topic")
	title = strings.Join(strings.Fields(title), " ")
	return title
}

func Classify(title string, duration time.Duration) Classification {
	normalized := normalizeForComparison(title)
	switch {
	case strings.Contains(normalized, "podcast") || strings.Contains(normalized, "episode") || strings.Contains(normalized, "interview"):
		return ClassPodcast
	case strings.Contains(normalized, "sermon") || strings.Contains(normalized, "speech") || strings.Contains(normalized, "lecture"):
		return ClassSpeech
	case strings.Contains(normalized, "nonstop mix") || strings.Contains(normalized, "dj mix"):
		return ClassLongMix
	case strings.Contains(normalized, "playlist") || strings.Contains(normalized, "mix"):
		if duration >= 15*time.Minute {
			return ClassPlaylistMix
		}
	case duration >= 20*time.Minute:
		return ClassLongMix
	case strings.Contains(normalized, "live at") || strings.Contains(normalized, " live ") || strings.HasSuffix(normalized, " live"):
		return ClassLivePerformance
	case strings.Contains(normalized, "cover"):
		return ClassCover
	case strings.Contains(normalized, "remix"):
		return ClassRemix
	case strings.Contains(normalized, "lyrics") || strings.Contains(normalized, "lyric video"):
		return ClassLyricsVideo
	case strings.Contains(normalized, "official audio"):
		return ClassOfficialTrack
	case strings.Contains(normalized, "official video") || strings.Contains(normalized, "music video"):
		return ClassMusicVideo
	default:
		return ClassUnknown
	}
	return ClassUnknown
}

func buildSourceComment(url, videoID, path string) string {
	parts := make([]string, 0, 3)
	if value := strings.TrimSpace(url); value != "" {
		parts = append(parts, "source_url="+value)
	}
	if value := strings.TrimSpace(videoID); value != "" {
		parts = append(parts, "video_id="+value)
	}
	if value := strings.TrimSpace(path); value != "" && strings.TrimSpace(url) == "" {
		parts = append(parts, "source_path="+filepath.Base(value))
	}
	return strings.Join(parts, " ")
}
