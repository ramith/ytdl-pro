package ytdlpro

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"ytdl-pro/internal/ytdlpro/metadata"
)

var metadataAudioExtensions = map[string]bool{
	".mp3":  true,
	".flac": true,
	".wav":  true,
	".m4a":  true,
	".mp4":  true,
	".ogg":  true,
	".opus": true,
	".webm": true,
	".aiff": true,
	".aif":  true,
}

func runMetadataPath(ctx context.Context, cfg Config, metadataSvc *metadata.Service) error {
	if metadataSvc == nil {
		return errors.New("metadata processing requires metadata mode to be enabled")
	}

	paths, err := collectMetadataPaths(cfg.Metadata.Path, cfg.Metadata.Recursive)
	if err != nil {
		return err
	}

	if len(paths) == 0 {
		return fmt.Errorf("no supported audio files found at %s", cfg.Metadata.Path)
	}

	if len(paths) == 1 {
		fmt.Printf("metadata target: %s\n", paths[0])
	} else {
		fmt.Printf("metadata scan: files=%d root=%s recursive=%t\n", len(paths), cfg.Metadata.Path, cfg.Metadata.Recursive)
	}

	failures := 0
	for i, path := range paths {
		if len(paths) > 1 {
			fmt.Printf("metadata file %d/%d: %s\n", i+1, len(paths), path)
		}
		result := metadataSvc.ProcessExistingPath(ctx, path)
		printMetadataResult(result, cfg.Explain, cfg.Debug)
		if result.Error != "" {
			failures++
		}
		if len(paths) > 1 {
			fmt.Println()
		}
	}

	if failures > 0 {
		return fmt.Errorf("metadata processing completed with %d failed file(s)", failures)
	}
	return nil
}

func collectMetadataPaths(root string, recursive bool) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("missing enrich path")
	}

	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat metadata path %s: %w", root, err)
	}

	if !info.IsDir() {
		if !isSupportedMetadataAudioPath(root) {
			return nil, fmt.Errorf("unsupported metadata file type: %s", root)
		}
		return []string{root}, nil
	}

	paths := []string{}
	if recursive {
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if isSupportedMetadataAudioPath(path) {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan metadata directory %s: %w", root, err)
		}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, fmt.Errorf("read metadata directory %s: %w", root, err)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(root, entry.Name())
			if isSupportedMetadataAudioPath(path) {
				paths = append(paths, path)
			}
		}
	}

	sort.Strings(paths)
	return paths, nil
}

func isSupportedMetadataAudioPath(path string) bool {
	return metadataAudioExtensions[strings.ToLower(filepath.Ext(path))]
}
