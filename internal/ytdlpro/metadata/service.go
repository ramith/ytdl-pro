package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	modelruntime "ytdl-pro/internal/ytdlpro/metadata/model"
	"ytdl-pro/internal/ytdlpro/tagging"
)

type cacheEntry struct {
	Candidates  []Candidate
	Decision    Decision
	Diagnostics Diagnostics
}

type Service struct {
	options        Options
	source         *musicBrainzClient
	modelClient    *modelruntime.Client
	modelInitError error

	mu     sync.Mutex
	cache  map[string]cacheEntry
	report JSONReport
}

func NewService(options Options) *Service {
	service := &Service{
		options: options,
		cache:   map[string]cacheEntry{},
	}
	if options.SourceMusicBrainz {
		service.source = newMusicBrainzClient()
	}
	switch strings.ToLower(strings.TrimSpace(options.Runtime)) {
	case "", "libllama":
		service.modelClient, service.modelInitError = modelruntime.NewClient(modelruntime.Config{
			Runtime:         options.Runtime,
			Model:           options.Model,
			ModelPath:       options.ModelPath,
			GrammarPath:     options.GrammarPath,
			ContextTokens:   options.ContextTokens,
			MaxOutputTokens: options.MaxOutputTokens,
			Temperature:     0,
			TopP:            1,
			Threads:         options.Threads,
			GPULayers:       options.GPULayers,
			Debug:           options.Debug,
			Explain:         options.Explain,
		})
	case "disabled", "none":
	}
	return service
}

func (s *Service) Enabled() bool {
	return s != nil
}

func (s *Service) Close() error {
	if s == nil || s.modelClient == nil {
		return nil
	}
	return s.modelClient.Close()
}

func (s *Service) Process(ctx context.Context, request Request) ItemReport {
	if s == nil {
		return ItemReport{}
	}

	if s.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.options.Timeout)
		defer cancel()
	}

	existingTags := tagging.Tags{}
	initialWarnings := []string{}
	if request.Path != "" {
		existingProbe, err := tagging.ReadTags(ctx, request.Path)
		if err != nil {
			initialWarnings = append(initialWarnings, "existing_tag_read_failed")
		} else {
			existingTags = existingProbe.Tags
		}
	}

	return s.processWithExisting(ctx, request, existingTags, initialWarnings)
}

func (s *Service) ProcessExistingPath(ctx context.Context, path string) ItemReport {
	if s == nil {
		return ItemReport{}
	}

	existingTags := tagging.Tags{}
	initialWarnings := []string{}
	duration := time.Duration(0)

	if s.options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.options.Timeout)
		defer cancel()
	}

	probe, err := tagging.ReadTags(ctx, path)
	if err != nil {
		initialWarnings = append(initialWarnings, "existing_tag_read_failed")
	} else {
		existingTags = probe.Tags
		duration = parseProbeDuration(probe.DurationText)
	}

	title := firstNonEmptyTag(existingTags.Title, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	channel := firstNonEmptyTag(existingTags.Artist, existingTags.AlbumArtist)
	request := Request{
		Path:     path,
		Title:    title,
		Channel:  channel,
		Duration: duration,
	}
	return s.processWithExisting(ctx, request, existingTags, initialWarnings)
}

func (s *Service) processWithExisting(ctx context.Context, request Request, existingTags tagging.Tags, initialWarnings []string) ItemReport {
	base := BuildBaseMetadata(request)
	report := ItemReport{
		Path:           request.Path,
		VideoID:        request.VideoID,
		Classification: string(base.Classification),
	}
	report.Warnings = append(report.Warnings, initialWarnings...)

	candidates, decision, diagnostics, warnings := s.resolve(ctx, base, existingTags)
	report.Warnings = append(report.Warnings, warnings...)
	report.Action = string(decision.Action)
	report.OverallConfidence = decision.OverallConfidence
	report.SelectedCandidateIDs = append(report.SelectedCandidateIDs, decision.SelectedCandidateIDs...)
	report.Diagnostics = diagnostics

	if s.options.ReviewOnly && (decision.Action == ActionWriteFull || decision.Action == ActionWritePartial || decision.Action == ActionWriteBase) {
		report.Action = string(ActionNeedsReview)
		report.Warnings = append(report.Warnings, "review_only_mode")
		s.record(report)
		return report
	}

	plannedTags, changedFields, skippedFields := buildWritePlan(base, existingTags, decision, s.options)
	report.ChangedFields = changedFields
	report.SkippedFields = skippedFields

	if len(changedFields) == 0 && (decision.Action == ActionWriteFull || decision.Action == ActionWritePartial || decision.Action == ActionWriteBase) {
		report.Action = string(ActionSkip)
		report.Warnings = append(report.Warnings, "metadata_already_matches_target")
	}

	if !s.options.Write || s.options.DryRun || report.Action == string(ActionSkip) || report.Action == string(ActionNeedsReview) {
		s.record(report)
		return report
	}

	if err := tagging.WriteTags(ctx, request.Path, plannedTags, tagging.WriteOptions{KeepBackup: s.options.KeepBackup}); err != nil {
		report.Error = err.Error()
		report.Warnings = append(report.Warnings, "tag_write_failed")
	}

	if len(candidates) == 0 && report.Action == string(ActionWriteBase) {
		report.Warnings = append(report.Warnings, "base_tags_only")
	}

	s.record(report)
	return report
}

func firstNonEmptyTag(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func parseProbeDuration(value string) time.Duration {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || seconds <= 0 {
		return 0
	}
	return time.Duration(seconds * float64(time.Second))
}

func (s *Service) resolve(ctx context.Context, base BaseMetadata, existing tagging.Tags) ([]Candidate, Decision, Diagnostics, []string) {
	cacheKey := s.cacheKey(base)
	useCache := len(existing.Values()) == 0
	if useCache {
		if cached, ok := s.getCache(cacheKey); ok {
			return cached.Candidates, cached.Decision, cached.Diagnostics, nil
		}
	}

	warnings := []string{}
	candidates := make([]Candidate, 0, 8)

	if existingCandidate := ExistingTagsCandidate(existing); existingCandidate != nil {
		candidates = append(candidates, *existingCandidate)
	}

	if s.source != nil && base.Classification != ClassPodcast && base.Classification != ClassSpeech && base.Classification != ClassPlaylistMix && base.Classification != ClassLongMix {
		found, err := s.lookupWithRetry(ctx, base)
		if err != nil {
			warnings = append(warnings, "musicbrainz_lookup_failed")
		} else {
			candidates = append(candidates, found...)
		}
	}

	candidates = ScoreCandidates(base, existing, dedupeCandidates(candidates))
	deterministic := deterministicDecision(base, candidates, s.options)
	decision := deterministic
	diagnostics := Diagnostics{
		CandidateCount:          len(candidates),
		TopCandidateIDs:         topCandidateIDs(candidates, 5),
		DeterministicAction:     string(deterministic.Action),
		DeterministicConfidence: deterministic.OverallConfidence,
		FinalDecisionSource:     "deterministic",
		Model: ModelDiagnostics{
			Backend: strings.TrimSpace(s.options.Runtime),
			Model:   strings.TrimSpace(s.options.Model),
		},
	}

	if len(candidates) > 0 && s.modelClient != nil {
		top := candidates
		if len(top) > 5 {
			top = top[:5]
		}
		modelDecision, modelDiagnostics, err := s.rankWithModel(ctx, base, existing, top)
		diagnostics.Model = modelDiagnostics
		if err != nil {
			warnings = append(warnings, "model_rank_fallback")
		} else {
			diagnostics.Model.Accepted = true
			decision = modelDecision
			diagnostics.FinalDecisionSource = "model"
		}
	} else {
		if len(candidates) == 0 {
			diagnostics.Model.FallbackReason = "no_candidates"
		} else if s.modelInitError != nil {
			diagnostics.Model.FallbackReason = "runtime_unavailable"
			diagnostics.Model.Error = s.modelInitError.Error()
		} else if s.modelClient == nil {
			diagnostics.Model.FallbackReason = "runtime_disabled"
		}
	}

	if useCache {
		s.putCache(cacheKey, cacheEntry{Candidates: candidates, Decision: decision, Diagnostics: diagnostics})
	}
	return candidates, decision, diagnostics, warnings
}

func (s *Service) rankWithModel(ctx context.Context, base BaseMetadata, existing tagging.Tags, candidates []Candidate) (Decision, ModelDiagnostics, error) {
	diagnostics := ModelDiagnostics{
		Backend:   strings.TrimSpace(s.options.Runtime),
		Model:     strings.TrimSpace(s.options.Model),
		Attempted: true,
	}

	inputJSON, err := json.MarshalIndent(buildModelInput(base, existing, candidates), "", "  ")
	if err != nil {
		diagnostics.FallbackReason = "input_marshal_error"
		diagnostics.Error = err.Error()
		return Decision{}, diagnostics, err
	}

	raw, err := s.modelClient.GenerateDecisionJSON(ctx, string(inputJSON))
	if err != nil {
		diagnostics.FallbackReason = "rank_error"
		diagnostics.Error = err.Error()
		return Decision{}, diagnostics, err
	}

	decision, err := decodeAndValidateDecision(raw, candidates)
	if err == nil {
		diagnostics.RawAction = string(decision.Action)
		diagnostics.RawOverallConfidence = decision.OverallConfidence
		diagnostics.RawSelectedCandidateIDs = append(diagnostics.RawSelectedCandidateIDs, decision.SelectedCandidateIDs...)
		return decision, diagnostics, nil
	}

	diagnostics.FallbackReason = "repair_attempted"
	diagnostics.Error = err.Error()

	raw, repairErr := s.modelClient.GenerateRepairJSON(ctx, string(inputJSON))
	if repairErr != nil {
		diagnostics.FallbackReason = "repair_runtime_error"
		diagnostics.Error = repairErr.Error()
		return Decision{}, diagnostics, repairErr
	}

	decision, err = decodeAndValidateDecision(raw, candidates)
	if err != nil {
		diagnostics.FallbackReason = "repair_validation_error"
		diagnostics.Error = err.Error()
		return Decision{}, diagnostics, err
	}

	diagnostics.RawAction = string(decision.Action)
	diagnostics.RawOverallConfidence = decision.OverallConfidence
	diagnostics.RawSelectedCandidateIDs = append(diagnostics.RawSelectedCandidateIDs, decision.SelectedCandidateIDs...)
	return decision, diagnostics, nil
}

func decodeAndValidateDecision(raw string, candidates []Candidate) (Decision, error) {
	var decision Decision
	decoder := json.NewDecoder(bytes.NewBufferString(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decision); err != nil {
		return Decision{}, fmt.Errorf("decode model JSON: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return Decision{}, fmt.Errorf("decode model JSON: trailing data")
	}
	if err := validateDecision(decision, candidates); err != nil {
		return Decision{}, fmt.Errorf("validate model JSON: %w", err)
	}
	return decision, nil
}

func buildModelInput(base BaseMetadata, existing tagging.Tags, candidates []Candidate) map[string]any {
	return map[string]any{
		"task": "rank_audio_metadata_candidates",
		"youtube": map[string]any{
			"title":               base.Title,
			"channel":             base.Channel,
			"playlist_title":      base.PlaylistTitle,
			"description_excerpt": truncateForModel(base.Description, 800),
			"duration_seconds":    int(base.Duration.Seconds()),
			"video_id":            base.VideoID,
			"url":                 base.URL,
		},
		"existing_tags":  existing.Values(),
		"classification": string(base.Classification),
		"candidates":     candidates,
	}
}

func truncateForModel(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	return value[:max]
}

func (s *Service) lookupWithRetry(ctx context.Context, base BaseMetadata) ([]Candidate, error) {
	var lastErr error
	for attempt := 0; attempt <= s.options.Retries; attempt++ {
		candidates, err := s.source.Lookup(ctx, base, 6)
		if err == nil {
			return candidates, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func dedupeCandidates(candidates []Candidate) []Candidate {
	seen := map[string]bool{}
	result := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if seen[candidate.CandidateID] {
			continue
		}
		seen[candidate.CandidateID] = true
		result = append(result, candidate)
	}
	return result
}

func buildWritePlan(base BaseMetadata, existing tagging.Tags, decision Decision, options Options) (tagging.Tags, []string, []string) {
	target := tagging.Tags{}
	switch decision.Action {
	case ActionWriteBase:
		target = base.BaseTags.Clone()
	case ActionWriteFull, ActionWritePartial:
		target.Comment = base.BaseTags.Comment
		if options.WriteBaseTags && existing.Date == "" && base.BaseTags.Date != "" {
			target.Date = base.BaseTags.Date
		}
		for field, decisionField := range decision.Fields {
			if decisionField.Value == nil || decisionField.Confidence < options.MinFieldConfidence {
				continue
			}
			target.Set(field, *decisionField.Value)
		}
		if target.Date == "" {
			target.Date = existing.Date
		}
	}
	if target.Date != "" && len(target.Date) >= 4 && target.Year == "" {
		target.Year = target.Date[:4]
	}

	changed := changedFields(existing, target)
	skipped := skippedFields(decision, options)
	return target, changed, skipped
}

func changedFields(existing, desired tagging.Tags) []string {
	values := desired.Values()
	fields := make([]string, 0, len(values))
	for field, value := range values {
		if strings.TrimSpace(existing.Get(field)) != strings.TrimSpace(value) {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

func skippedFields(decision Decision, options Options) []string {
	fields := []string{}
	for field, decisionField := range decision.Fields {
		if decisionField.Value == nil || decisionField.Confidence < options.MinFieldConfidence {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	return fields
}

func (s *Service) cacheKey(base BaseMetadata) string {
	durationBucket := int(base.Duration.Seconds()) / 5
	return strings.Join([]string{
		normalizeForComparison(base.SearchTitle),
		normalizeForComparison(base.Channel),
		fmt.Sprintf("%d", durationBucket),
	}, "|")
}

func (s *Service) getCache(key string) (cacheEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cache[key]
	return entry, ok
}

func (s *Service) putCache(key string, entry cacheEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[key] = entry
}

func (s *Service) record(item ItemReport) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.report.Items = append(s.report.Items, item)
	s.report.Summary.FilesScanned++
	if item.Error != "" {
		s.report.Summary.Errors++
	}
	if item.Diagnostics.Model.Attempted {
		s.report.Summary.ModelAttempted++
		if item.Diagnostics.Model.Accepted {
			s.report.Summary.ModelAccepted++
		} else {
			s.report.Summary.ModelFallbacks++
		}
	} else {
		s.report.Summary.ModelUnused++
	}
	switch item.Action {
	case string(ActionWriteBase):
		if item.Error == "" && !s.options.DryRun && s.options.Write && len(item.ChangedFields) > 0 {
			s.report.Summary.FilesChanged++
			s.report.Summary.FilesBaseTagged++
		}
	case string(ActionWriteFull), string(ActionWritePartial):
		if item.Error == "" && !s.options.DryRun && s.options.Write && len(item.ChangedFields) > 0 {
			s.report.Summary.FilesChanged++
		}
	case string(ActionNeedsReview):
		s.report.Summary.NeedsReview++
	default:
		s.report.Summary.FilesSkipped++
	}
}

func topCandidateIDs(candidates []Candidate, limit int) []string {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	if len(candidates) < limit {
		limit = len(candidates)
	}
	ids := make([]string, 0, limit)
	for _, candidate := range candidates[:limit] {
		ids = append(ids, candidate.CandidateID)
	}
	return ids
}

func (s *Service) WriteReport() error {
	if s == nil || strings.TrimSpace(s.options.JSONReport) == "" {
		return nil
	}

	path := s.options.JSONReport
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create metadata report directory: %w", err)
	}

	data, err := json.MarshalIndent(s.report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata report: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("write metadata report: %w", err)
	}
	return nil
}
