package ytdlpro

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/kkdai/youtube/v2"
)

var playlistIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{13,42}$`)

func Run(ctx context.Context, cfg Config) error {
	client := youtube.Client{}

	if cfg.Playlist || IsPlaylistURL(cfg.URL) {
		return runPlaylist(ctx, &client, cfg)
	}

	video, err := client.GetVideoContext(ctx, cfg.URL)
	if err != nil {
		return fmt.Errorf("load video metadata: %w", err)
	}

	if cfg.ListFormats {
		PrintFormats(os.Stdout, video)
		return nil
	}

	if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", cfg.OutDir, err)
	}

	return downloadVideo(ctx, &client, video, cfg)
}

func runPlaylist(ctx context.Context, client *youtube.Client, cfg Config) error {
	if strings.TrimSpace(cfg.Filename) != "" {
		return errors.New("-filename cannot be used with a playlist; filenames come from each video title")
	}

	playlistCtx, cancelPlaylist := operationContext(ctx, cfg.Timeout)
	playlist, err := client.GetPlaylistContext(playlistCtx, cfg.URL)
	cancelPlaylist()
	if err != nil {
		return fmt.Errorf("load playlist metadata: %w", err)
	}

	if cfg.ListFormats {
		PrintPlaylist(os.Stdout, playlist)
		return nil
	}

	if err := os.MkdirAll(cfg.OutDir, 0755); err != nil {
		return fmt.Errorf("create output directory %q: %w", cfg.OutDir, err)
	}

	total := len(playlist.Videos)
	fmt.Printf("playlist: %s\n", playlist.Title)
	fmt.Printf("items: %d\n\n", total)

	var failures []error
	completed := 0
	for i, entry := range playlist.Videos {
		if err := ctx.Err(); err != nil {
			return err
		}

		fmt.Printf("playlist item %d/%d: %s\n", i+1, total, entry.Title)
		itemCtx, cancelItem := operationContext(ctx, cfg.Timeout)
		video, err := client.GetVideoContext(itemCtx, entry.ID)
		if err == nil {
			err = downloadVideo(itemCtx, client, video, cfg)
		}
		cancelItem()

		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			wrapped := fmt.Errorf("item %d %q: %w", i+1, entry.Title, err)
			failures = append(failures, wrapped)
			fmt.Fprintln(os.Stderr, "error:", wrapped)
		} else {
			completed++
		}
		fmt.Println()
	}

	fmt.Printf("playlist complete: downloaded=%d failed=%d total=%d\n", completed, len(failures), total)
	if len(failures) > 0 {
		return fmt.Errorf("playlist completed with %d failed item(s)", len(failures))
	}
	return nil
}

func downloadVideo(ctx context.Context, client *youtube.Client, video *youtube.Video, cfg Config) error {
	dl := Downloader{Client: client}
	if cfg.AudioOnly {
		return dl.DownloadAudio(ctx, video, cfg)
	}
	return dl.DownloadVideo(ctx, video, cfg)
}

func operationContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, timeout)
}

func IsPlaylistURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if playlistIDPattern.MatchString(raw) && !IsRadioMixID(raw) {
		return true
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}

	playlistID := parsed.Query().Get("list")
	if IsRadioMixID(playlistID) {
		return false
	}

	return playlistID != "" || strings.EqualFold(strings.Trim(parsed.Path, "/"), "playlist")
}

func IsRadioMixID(id string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(id)), "RD")
}
