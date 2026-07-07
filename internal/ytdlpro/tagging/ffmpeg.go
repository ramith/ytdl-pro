package tagging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type WriteOptions struct {
	KeepBackup bool
}

func RequireFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg not found; install it with: brew install ffmpeg")
	}
	return nil
}

func WriteTags(ctx context.Context, path string, tags Tags, options WriteOptions) error {
	if tags.Empty() {
		return nil
	}
	if err := RequireFFmpeg(); err != nil {
		return err
	}
	if err := RequireFFprobe(); err != nil {
		return err
	}

	muxer, err := muxerForPath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(path)
	pattern := "." + filepath.Base(path) + ".*.tags"
	tmp, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return fmt.Errorf("create temp tag output: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp tag output: %w", err)
	}
	defer os.Remove(tmpPath)

	args := []string{"-hide_banner", "-y", "-i", path, "-map", "0", "-c", "copy", "-map_metadata", "0"}
	for _, pair := range metadataPairs(tags) {
		args = append(args, "-metadata", pair)
	}
	args = append(args, "-f", muxer, tmpPath)

	if err := runFFmpeg(ctx, args); err != nil {
		return err
	}

	probe, err := ReadTags(ctx, tmpPath)
	if err != nil {
		return fmt.Errorf("verify rewritten tags: %w", err)
	}
	if err := verifyTags(probe, tags); err != nil {
		return err
	}

	if options.KeepBackup {
		backupPath, err := nextBackupPath(path)
		if err != nil {
			return err
		}
		if err := copyFile(path, backupPath); err != nil {
			return fmt.Errorf("create metadata backup: %w", err)
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace original with retagged file: %w", err)
	}
	return nil
}

func metadataPairs(tags Tags) []string {
	values := tags.Values()
	pairs := make([]string, 0, len(values)+2)
	for _, item := range []struct {
		field string
		keys  []string
	}{
		{"title", []string{"title"}},
		{"artist", []string{"artist"}},
		{"album", []string{"album"}},
		{"album_artist", []string{"album_artist"}},
		{"date", []string{"date"}},
		{"year", []string{"year"}},
		{"genre", []string{"genre"}},
		{"comment", []string{"comment"}},
		{"label", []string{"label", "publisher"}},
		{"track_number", []string{"track"}},
		{"disc_number", []string{"disc"}},
		{"musicbrainz_recording_id", []string{"musicbrainz_trackid"}},
		{"musicbrainz_release_id", []string{"musicbrainz_albumid"}},
	} {
		value := values[item.field]
		if value == "" {
			continue
		}
		for _, key := range item.keys {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, value))
		}
	}
	return pairs
}

func verifyTags(probe ProbeResult, expected Tags) error {
	for field, want := range expected.Values() {
		got := probe.Tags.Get(field)
		if field == "label" && got == "" {
			got = firstNonEmpty(probe.RawTags["publisher"], probe.RawTags["organization"])
		}
		if strings.TrimSpace(got) != strings.TrimSpace(want) {
			return fmt.Errorf("ffprobe verification failed for %s: got %q want %q", field, got, want)
		}
	}
	return nil
}

func muxerForPath(path string) (string, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return "mp3", nil
	case ".flac":
		return "flac", nil
	case ".wav":
		return "wav", nil
	case ".m4a":
		return "ipod", nil
	case ".mp4":
		return "mp4", nil
	case ".mkv":
		return "matroska", nil
	case ".webm":
		return "webm", nil
	case ".ogg":
		return "ogg", nil
	case ".opus":
		return "opus", nil
	default:
		return "", fmt.Errorf("cannot determine metadata muxer for %q", path)
	}
}

func nextBackupPath(path string) (string, error) {
	candidate := path + ".bak"
	if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate, nil
	} else if err != nil {
		return "", fmt.Errorf("check backup path %s: %w", candidate, err)
	}

	for suffix := 1; ; suffix++ {
		candidate = fmt.Sprintf("%s.%d", path+".bak", suffix)
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil {
			return "", fmt.Errorf("check backup path %s: %w", candidate, err)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, tail(stderr.String(), 2000))
	}
	return nil
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
