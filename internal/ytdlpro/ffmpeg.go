package ytdlpro

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type FFmpeg struct {
	Overwrite bool
}

func RequireFFmpeg() error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return errors.New("ffmpeg not found; install it with: brew install ffmpeg")
	}
	return nil
}

func (f FFmpeg) TranscodeAudio(ctx context.Context, inputPath, outputPath string, cfg Config) error {
	muxer, err := FFmpegMuxerForPath(outputPath)
	if err != nil {
		return err
	}

	return WriteByTempOutput(outputPath, cfg.Overwrite, func(tmpOut string) error {
		args := f.baseArgs()
		args = append(args, "-i", inputPath, "-vn", "-map", "0:a:0")

		switch cfg.AudioFormat {
		case AudioMP3:
			args = append(args, "-codec:a", "libmp3lame")
			if cfg.MP3Mode == MP3VBR {
				args = append(args, "-q:a", strconv.Itoa(cfg.MP3VBR), "-compression_level:a", "0")
			} else {
				bitrate, err := NormalizeBitrate(cfg.MP3Bitrate)
				if err != nil {
					return err
				}
				args = append(args, "-b:a", bitrate)
			}
		case AudioFLAC:
			args = append(args, "-codec:a", "flac")
		case AudioWAV:
			args = append(args, "-codec:a", "pcm_s16le")
		case AudioALAC:
			args = append(args, "-codec:a", "alac")
		default:
			return fmt.Errorf("unsupported audio conversion format %q", cfg.AudioFormat)
		}

		args = append(args, "-f", muxer, tmpOut)
		return runFFmpeg(ctx, args)
	})
}

func (f FFmpeg) Mux(ctx context.Context, videoPath, audioPath, outputPath string) error {
	muxer, err := FFmpegMuxerForPath(outputPath)
	if err != nil {
		return err
	}

	return WriteByTempOutput(outputPath, f.Overwrite, func(tmpOut string) error {
		args := f.baseArgs()
		args = append(args,
			"-i", videoPath,
			"-i", audioPath,
			"-map", "0:v:0",
			"-map", "1:a:0",
			"-c", "copy",
			"-shortest",
			"-f", muxer,
			tmpOut,
		)
		return runFFmpeg(ctx, args)
	})
}

func FFmpegMuxerForPath(path string) (string, error) {
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
	default:
		return "", fmt.Errorf("cannot determine ffmpeg output format for %q", path)
	}
}

func (f FFmpeg) baseArgs() []string {
	// Atomic output paths are pre-created and safe to replace. The final output
	// collision policy is enforced before FFmpeg runs.
	return []string{"-hide_banner", "-y"}
}

func runFFmpeg(ctx context.Context, args []string) error {
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg failed: %w: %s", err, tail(stderr.String(), 2000))
	}

	if output := strings.TrimSpace(stderr.String()); output != "" {
		fmt.Println(tail(output, 1000))
	}
	return nil
}

func tail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[len(s)-max:]
}
