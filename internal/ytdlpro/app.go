package ytdlpro

import (
	"context"
	"fmt"
	"os"

	"github.com/kkdai/youtube/v2"
)

func Run(ctx context.Context, cfg Config) error {
	client := youtube.Client{}

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

	dl := Downloader{Client: &client}

	if cfg.AudioOnly {
		return dl.DownloadAudio(ctx, video, cfg)
	}

	return dl.DownloadVideo(ctx, video, cfg)
}
