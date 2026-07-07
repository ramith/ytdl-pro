package tagging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type ffprobeOutput struct {
	Format struct {
		Duration string            `json:"duration"`
		Tags     map[string]string `json:"tags"`
	} `json:"format"`
}

type ProbeResult struct {
	Tags         Tags
	RawTags      map[string]string
	DurationText string
}

func RequireFFprobe() error {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return errors.New("ffprobe not found; install it with: brew install ffmpeg")
	}
	return nil
}

func ReadTags(ctx context.Context, path string) (ProbeResult, error) {
	if err := RequireFFprobe(); err != nil {
		return ProbeResult{}, err
	}

	args := []string{
		"-v", "error",
		"-print_format", "json",
		"-show_entries", "format=duration:format_tags",
		path,
	}

	cmd := exec.CommandContext(ctx, "ffprobe", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return ProbeResult{}, fmt.Errorf("ffprobe failed: %w: %s", err, tail(strings.TrimSpace(stderr.String()), 1000))
	}

	var out ffprobeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return ProbeResult{}, fmt.Errorf("decode ffprobe JSON: %w", err)
	}

	lowered := map[string]string{}
	for key, value := range out.Format.Tags {
		lowered[strings.ToLower(strings.TrimSpace(key))] = strings.TrimSpace(value)
	}

	tags := Tags{
		Title:                  firstNonEmpty(lowered["title"]),
		Artist:                 firstNonEmpty(lowered["artist"]),
		Album:                  firstNonEmpty(lowered["album"]),
		AlbumArtist:            firstNonEmpty(lowered["album_artist"], lowered["albumartist"]),
		Date:                   firstNonEmpty(lowered["date"]),
		Year:                   firstNonEmpty(lowered["year"]),
		Genre:                  firstNonEmpty(lowered["genre"]),
		Comment:                firstNonEmpty(lowered["comment"], lowered["description"]),
		Label:                  firstNonEmpty(lowered["label"], lowered["publisher"], lowered["organization"]),
		TrackNumber:            firstNonEmpty(lowered["track"], lowered["tracknumber"]),
		DiscNumber:             firstNonEmpty(lowered["disc"], lowered["discnumber"]),
		MusicBrainzRecordingID: firstNonEmpty(lowered["musicbrainz_trackid"], lowered["musicbrainz_recordingid"]),
		MusicBrainzReleaseID:   firstNonEmpty(lowered["musicbrainz_albumid"], lowered["musicbrainz_releaseid"]),
	}

	return ProbeResult{
		Tags:         tags,
		RawTags:      lowered,
		DurationText: strings.TrimSpace(out.Format.Duration),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
