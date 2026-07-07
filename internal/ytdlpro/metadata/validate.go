package metadata

import (
	"fmt"
	"strings"
)

func validateDecision(decision Decision, candidates []Candidate) error {
	allowedActions := map[Action]bool{
		ActionWriteFull:    true,
		ActionWritePartial: true,
		ActionWriteBase:    true,
		ActionSkip:         true,
		ActionNeedsReview:  true,
	}
	if !allowedActions[decision.Action] {
		return fmt.Errorf("invalid action %q", decision.Action)
	}
	if decision.OverallConfidence < 0 || decision.OverallConfidence > 1 {
		return fmt.Errorf("overall confidence out of range")
	}

	knownCandidates := map[string]bool{}
	for _, candidate := range candidates {
		knownCandidates[candidate.CandidateID] = true
	}

	for _, selected := range decision.SelectedCandidateIDs {
		if !knownCandidates[selected] {
			return fmt.Errorf("unknown selected candidate %q", selected)
		}
	}

	allowedFields := map[string]bool{
		"title":                    true,
		"artist":                   true,
		"album":                    true,
		"album_artist":             true,
		"date":                     true,
		"genre":                    true,
		"label":                    true,
		"track_number":             true,
		"disc_number":              true,
		"musicbrainz_recording_id": true,
		"musicbrainz_release_id":   true,
	}

	for field, decisionField := range decision.Fields {
		if !allowedFields[field] {
			return fmt.Errorf("unknown field %q", field)
		}
		if decisionField.Confidence < 0 || decisionField.Confidence > 1 {
			return fmt.Errorf("field confidence out of range for %s", field)
		}
		if decisionField.Value != nil {
			if decisionField.SourceCandidateID == nil || strings.TrimSpace(*decisionField.SourceCandidateID) == "" {
				return fmt.Errorf("field %s is missing source_candidate_id", field)
			}
			if !knownCandidates[*decisionField.SourceCandidateID] {
				return fmt.Errorf("field %s references unknown candidate %q", field, *decisionField.SourceCandidateID)
			}
		}
	}

	for field := range allowedFields {
		if _, ok := decision.Fields[field]; !ok {
			return fmt.Errorf("missing field %q", field)
		}
	}
	return nil
}
