package ytdlpro

import (
	"fmt"
	"path/filepath"
	"strings"

	"ytdl-pro/internal/ytdlpro/metadata"
)

func printMetadataResult(result metadata.ItemReport, explain, debug bool) {
	status := metadataStatusLabel(result)
	label := filepath.Base(result.Path)
	if strings.TrimSpace(label) == "" {
		label = result.Path
	}
	fmt.Printf("%s: %s\n", status, label)

	if explain || debug {
		fmt.Printf("  confidence=%.2f decision=%s source=%s\n",
			result.OverallConfidence,
			result.Action,
			result.Diagnostics.FinalDecisionSource,
		)
		if len(result.SelectedCandidateIDs) > 0 {
			fmt.Printf("  selected=%s\n", strings.Join(result.SelectedCandidateIDs, ", "))
		}
		if reason := strings.TrimSpace(result.Diagnostics.Model.FallbackReason); reason != "" {
			fmt.Printf("  fallback=%s\n", reason)
		}
		if err := strings.TrimSpace(result.Diagnostics.Model.Error); err != "" {
			fmt.Printf("  runtime=%s\n", err)
		}
	}

	if result.Error != "" {
		fmt.Println("  failed:", result.Error)
	}
}

func metadataStatusLabel(result metadata.ItemReport) string {
	if result.Error != "" || result.Action == string(metadata.ActionSkip) && len(result.ChangedFields) == 0 && len(result.Warnings) > 0 && containsWarning(result.Warnings, "tag_write_failed") {
		return "failed"
	}
	switch result.Action {
	case string(metadata.ActionWriteFull):
		return "enriched"
	case string(metadata.ActionWritePartial):
		return "partially enriched"
	case string(metadata.ActionWriteBase):
		return "base tagged"
	case string(metadata.ActionSkip), string(metadata.ActionNeedsReview):
		return "skipped"
	default:
		return "skipped"
	}
}

func containsWarning(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
