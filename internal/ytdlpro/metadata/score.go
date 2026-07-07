package metadata

import (
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
	"ytdl-pro/internal/ytdlpro/tagging"
)

func ScoreCandidates(base BaseMetadata, existing tagging.Tags, candidates []Candidate) []Candidate {
	scored := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		candidate.Evidence = EvidenceBreakdown{
			TitleMatch:       similarity(base.SearchTitle, candidate.Title),
			ArtistMatch:      similarity(base.SearchArtist, candidate.Artist),
			DurationMatch:    durationSimilarity(int(base.Duration.Seconds()), candidate.DurationSeconds),
			AlbumMatch:       albumSimilarity(base, existing, candidate),
			TrackNumberMatch: trackSimilarity(existing, candidate),
		}
		candidate.PreScore =
			0.30*candidate.Evidence.TitleMatch +
				0.20*candidate.Evidence.ArtistMatch +
				0.15*candidate.Evidence.DurationMatch +
				0.15*candidate.Evidence.AlbumMatch +
				0.10*candidate.Evidence.TrackNumberMatch +
				0.10*candidate.SourceTrust
		scored = append(scored, candidate)
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].PreScore == scored[j].PreScore {
			return scored[i].SourceTrust > scored[j].SourceTrust
		}
		return scored[i].PreScore > scored[j].PreScore
	})
	return scored
}

func similarity(left, right string) float64 {
	left = normalizeForComparison(left)
	right = normalizeForComparison(right)
	if left == "" || right == "" {
		return 0
	}
	if left == right {
		return 1
	}
	if strings.Contains(left, right) || strings.Contains(right, left) {
		return 0.92
	}

	leftTokens := tokenize(left)
	rightTokens := tokenize(right)
	if len(leftTokens) == 0 || len(rightTokens) == 0 {
		return 0
	}

	rightCounts := map[string]int{}
	for _, token := range rightTokens {
		rightCounts[token]++
	}

	intersection := 0
	for _, token := range leftTokens {
		if rightCounts[token] > 0 {
			intersection++
			rightCounts[token]--
		}
	}

	return clamp((2 * float64(intersection)) / float64(len(leftTokens)+len(rightTokens)))
}

func tokenize(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		result = append(result, field)
	}
	return result
}

func durationSimilarity(baseSeconds, candidateSeconds int) float64 {
	if baseSeconds <= 0 || candidateSeconds <= 0 {
		return 0.5
	}
	diff := abs(baseSeconds - candidateSeconds)
	switch {
	case diff <= 2:
		return 1.0
	case diff <= 5:
		return 0.8
	case diff <= 10:
		return 0.5
	default:
		return 0
	}
}

func albumSimilarity(base BaseMetadata, existing tagging.Tags, candidate Candidate) float64 {
	reference := strings.TrimSpace(existing.Album)
	if reference == "" {
		reference = strings.TrimSpace(base.PlaylistTitle)
	}
	if reference == "" || strings.TrimSpace(candidate.Album) == "" {
		return 0.5
	}
	return similarity(reference, candidate.Album)
}

func trackSimilarity(existing tagging.Tags, candidate Candidate) float64 {
	if existing.TrackNumber == "" || candidate.TrackNumber <= 0 {
		return 0.5
	}
	current, err := strconv.Atoi(strings.TrimSpace(existing.TrackNumber))
	if err != nil {
		return 0.5
	}
	if current == candidate.TrackNumber {
		return 1.0
	}
	return 0
}

func normalizeForComparison(value string) string {
	value = stripControlCharacters(value)
	value = norm.NFKC.String(value)
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(
		"_", " ",
		"-", " ",
		"/", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		",", " ",
		".", " ",
		"!", " ",
		"?", " ",
		":", " ",
	).Replace(value)
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func stripControlCharacters(value string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, value)
}

func clamp(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 1:
		return 1
	default:
		return value
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func deterministicDecision(base BaseMetadata, candidates []Candidate, options Options) Decision {
	if len(candidates) == 0 {
		return Decision{
			Action:            fallbackAction(options),
			ReasonCodes:       []string{"no_candidates"},
			Fields:            map[string]FieldDecision{},
			Warnings:          nil,
			OverallConfidence: 0,
		}
	}

	top := candidates[0]
	overall := clamp(top.PreScore)
	fields := map[string]FieldDecision{}

	for field, confidence := range map[string]float64{
		"title":                    clamp(0.55*top.Evidence.TitleMatch + 0.25*top.Evidence.ArtistMatch + 0.20*top.SourceTrust),
		"artist":                   clamp(0.55*top.Evidence.ArtistMatch + 0.20*top.Evidence.TitleMatch + 0.25*top.SourceTrust),
		"album":                    clamp(0.55*top.Evidence.AlbumMatch + 0.20*top.Evidence.ArtistMatch + 0.25*top.SourceTrust),
		"album_artist":             clamp(0.55*top.Evidence.ArtistMatch + 0.20*top.Evidence.AlbumMatch + 0.25*top.SourceTrust),
		"date":                     clamp(0.40*top.Evidence.TitleMatch + 0.20*top.Evidence.ArtistMatch + 0.40*top.SourceTrust),
		"genre":                    0,
		"label":                    0,
		"track_number":             clamp(0.50*top.Evidence.TrackNumberMatch + 0.50*top.SourceTrust),
		"disc_number":              clamp(0.35 + 0.65*top.SourceTrust),
		"musicbrainz_recording_id": clamp(0.20 + 0.80*top.SourceTrust),
		"musicbrainz_release_id":   clamp(0.20 + 0.75*top.SourceTrust),
	} {
		value := top.ToTags().Get(field)
		if value == "" {
			fields[field] = FieldDecision{Confidence: 0}
			continue
		}
		valueCopy := value
		sourceID := top.CandidateID
		fields[field] = FieldDecision{
			Value:             &valueCopy,
			Confidence:        confidence,
			SourceCandidateID: &sourceID,
		}
	}

	action := decideAction(base.Classification, overall, fields, options)
	return Decision{
		Action:               action,
		OverallConfidence:    overall,
		SelectedCandidateIDs: []string{top.CandidateID},
		ReasonCodes:          []string{"deterministic_score"},
		Fields:               fields,
	}
}

func decideAction(classification Classification, overall float64, fields map[string]FieldDecision, options Options) Action {
	if classification == ClassPodcast || classification == ClassSpeech || classification == ClassPlaylistMix || classification == ClassLongMix {
		return fallbackAction(options)
	}

	fullThreshold := options.MinFullConfidence
	fieldThreshold := options.MinFieldConfidence
	reviewThreshold := options.MinReviewConfidence

	if classification == ClassLivePerformance || classification == ClassCover || classification == ClassRemix {
		fullThreshold = math.Min(1, fullThreshold+0.05)
		fieldThreshold = math.Min(1, fieldThreshold+0.05)
	}
	if classification == ClassUnknown {
		fieldThreshold = fullThreshold
	}

	highConfidenceFields := 0
	for _, field := range []string{"title", "artist", "album"} {
		if fields[field].Confidence >= fieldThreshold && fields[field].Value != nil {
			highConfidenceFields++
		}
	}

	switch {
	case overall >= fullThreshold && highConfidenceFields >= 2:
		return ActionWriteFull
	case overall >= fieldThreshold && highConfidenceFields >= 2:
		return ActionWritePartial
	case overall >= reviewThreshold:
		return ActionNeedsReview
	default:
		return fallbackAction(options)
	}
}

func fallbackAction(options Options) Action {
	if options.WriteBaseTags {
		return ActionWriteBase
	}
	return ActionSkip
}
