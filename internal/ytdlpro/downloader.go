package ytdlpro

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/kkdai/youtube/v2"
)

type Downloader struct {
	Client *youtube.Client
}

func (d Downloader) DownloadAudio(ctx context.Context, video *youtube.Video, cfg Config) error {
	source, err := SelectAudioFormat(video, cfg.AudioQuality)
	if err != nil {
		return err
	}

	resolvedFormat := cfg.AudioFormat
	if cfg.AudioFormat == AudioSmart {
		if isLosslessM4A(source) {
			resolvedFormat = AudioOriginal
		} else {
			resolvedFormat = AudioMP3
		}
	}

	if resolvedFormat == AudioOriginal {
		outPath, err := ResolveOutputPath(cfg.OutDir, cfg.Filename, video.Title, ExtensionForMIME(source.MimeType, ".audio"), cfg.Overwrite)
		if err != nil {
			return err
		}
		PrintSelectedAudio(source, outputLabel(cfg.AudioFormat, resolvedFormat, source))

		written, err := d.downloadFormatAtomically(ctx, video, source, outPath, cfg.Overwrite)
		if err != nil {
			return err
		}

		fmt.Printf("written bytes=%d\n", written)
		fmt.Println("downloaded:", outPath)
		return nil
	}

	if err := RequireFFmpeg(); err != nil {
		return err
	}

	outPath, err := ResolveOutputPath(cfg.OutDir, cfg.Filename, video.Title, AudioFormatExtension(resolvedFormat), cfg.Overwrite)
	if err != nil {
		return err
	}
	if err := EnsureCanWrite(outPath, cfg.Overwrite); err != nil {
		return err
	}

	sourcePath, cleanup, err := CreateTempPath(cfg.OutDir, "audio-source-*"+ExtensionForMIME(source.MimeType, ".audio"))
	if err != nil {
		return err
	}
	defer cleanup()

	PrintSelectedAudio(source, outputLabel(cfg.AudioFormat, resolvedFormat, source))

	written, err := d.downloadFormatAtomically(ctx, video, source, sourcePath, true)
	if err != nil {
		return err
	}
	fmt.Printf("source bytes=%d\n", written)

	ff := FFmpeg{Overwrite: cfg.Overwrite}
	transcodeCfg := cfg
	transcodeCfg.AudioFormat = resolvedFormat
	if err := ff.TranscodeAudio(ctx, sourcePath, outPath, transcodeCfg); err != nil {
		return err
	}

	fmt.Println("downloaded:", outPath)
	return nil
}

func isLosslessM4A(format *youtube.Format) bool {
	mime := strings.ToLower(format.MimeType)
	return strings.Contains(mime, "audio/mp4") && strings.Contains(mime, "alac")
}

func outputLabel(requested AudioFormat, resolved AudioFormat, source *youtube.Format) string {
	if requested != AudioSmart {
		return string(resolved)
	}
	if resolved == AudioOriginal {
		return "smart-lossless-m4a"
	}
	return "smart-mp3"
}

func (d Downloader) DownloadVideo(ctx context.Context, video *youtube.Video, cfg Config) error {
	selection, err := SelectVideoFormat(video, cfg.VideoQuality)
	if err != nil {
		return err
	}

	outPath, err := ResolveOutputPath(cfg.OutDir, cfg.Filename, video.Title, selection.OutputExt, cfg.Overwrite)
	if err != nil {
		return err
	}

	if !selection.Merge {
		PrintSelectedVideo(selection.VideoFormat, false)

		written, err := d.downloadFormatAtomically(ctx, video, selection.VideoFormat, outPath, cfg.Overwrite)
		if err != nil {
			return err
		}

		fmt.Printf("written bytes=%d\n", written)
		fmt.Println("downloaded:", outPath)
		return nil
	}

	if err := RequireFFmpeg(); err != nil {
		return err
	}

	if err := EnsureCanWrite(outPath, cfg.Overwrite); err != nil {
		return err
	}

	videoTmp, cleanupVideo, err := CreateTempPath(cfg.OutDir, "video-source-*"+ExtensionForMIME(selection.VideoFormat.MimeType, ".video"))
	if err != nil {
		return err
	}
	defer cleanupVideo()

	audioTmp, cleanupAudio, err := CreateTempPath(cfg.OutDir, "audio-source-*"+ExtensionForMIME(selection.AudioFormat.MimeType, ".audio"))
	if err != nil {
		return err
	}
	defer cleanupAudio()

	PrintSelectedVideo(selection.VideoFormat, true)
	PrintSelectedAudio(selection.AudioFormat, "mux")

	if _, err := d.downloadFormatAtomically(ctx, video, selection.VideoFormat, videoTmp, true); err != nil {
		return err
	}

	if _, err := d.downloadFormatAtomically(ctx, video, selection.AudioFormat, audioTmp, true); err != nil {
		return err
	}

	ff := FFmpeg{Overwrite: cfg.Overwrite}
	if err := ff.Mux(ctx, videoTmp, audioTmp, outPath); err != nil {
		return err
	}

	fmt.Println("downloaded:", outPath)
	return nil
}

func (d Downloader) downloadFormatAtomically(ctx context.Context, video *youtube.Video, format *youtube.Format, outPath string, overwrite bool) (int64, error) {
	return WriteAtomically(outPath, overwrite, func(tmpPath string) (int64, error) {
		stream, expectedSize, err := d.Client.GetStreamContext(ctx, video, format)
		if err != nil {
			return 0, fmt.Errorf("open stream for itag %d: %w", format.ItagNo, err)
		}
		defer stream.Close()

		out, err := OpenTruncate(tmpPath)
		if err != nil {
			return 0, err
		}

		written, copyErr := io.Copy(out, stream)
		closeErr := out.Close()

		if copyErr != nil {
			return written, fmt.Errorf("write stream to %s: %w", filepath.Base(tmpPath), copyErr)
		}
		if closeErr != nil {
			return written, fmt.Errorf("close %s: %w", filepath.Base(tmpPath), closeErr)
		}

		if expectedSize > 0 && written != expectedSize {
			return written, fmt.Errorf("partial download for itag %d: expected %d bytes, wrote %d bytes", format.ItagNo, expectedSize, written)
		}

		return written, nil
	})
}
