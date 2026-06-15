package ytdlpro

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func BuildOutputPath(outDir, filename, title, ext string) string {
	name := strings.TrimSpace(filename)
	if name == "" {
		name = SafeFilename(title) + ext
	} else if filepath.Ext(name) == "" {
		name += ext
	}
	return filepath.Join(outDir, name)
}

func SafeFilename(name string) string {
	replacer := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)

	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" {
		return "youtube-download"
	}
	if len(name) > 160 {
		name = name[:160]
	}
	return name
}

func EnsureCanWrite(path string, overwrite bool) error {
	if overwrite {
		return nil
	}

	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("output exists: %s; use -overwrite to replace it", path)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("check output path %s: %w", path, err)
	}
	return nil
}

func OpenTruncate(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return nil, fmt.Errorf("create %s: %w", path, err)
	}
	return file, nil
}

func WriteAtomically(finalPath string, overwrite bool, write func(tmpPath string) (int64, error)) (int64, error) {
	if err := EnsureCanWrite(finalPath, overwrite); err != nil {
		return 0, err
	}

	dir := filepath.Dir(finalPath)
	base := "." + filepath.Base(finalPath) + ".*.part"
	tmp, err := os.CreateTemp(dir, base)
	if err != nil {
		return 0, fmt.Errorf("create temp file in %s: %w", dir, err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return 0, fmt.Errorf("close temp file: %w", err)
	}
	defer os.Remove(tmpPath)

	written, err := write(tmpPath)
	if err != nil {
		return written, err
	}

	if overwrite {
		if err := os.Remove(finalPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return written, fmt.Errorf("remove existing output %s: %w", finalPath, err)
		}
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		return written, fmt.Errorf("rename temp file to %s: %w", finalPath, err)
	}
	return written, nil
}

func WriteByTempOutput(finalPath string, overwrite bool, write func(tmpOut string) error) error {
	_, err := WriteAtomically(finalPath, overwrite, func(tmpPath string) (int64, error) {
		if err := write(tmpPath); err != nil {
			return 0, err
		}

		info, err := os.Stat(tmpPath)
		if err != nil {
			return 0, fmt.Errorf("stat temp output: %w", err)
		}
		if info.Size() == 0 {
			return 0, errors.New("encoder produced an empty output file")
		}
		return info.Size(), nil
	})
	return err
}

func CreateTempPath(dir, pattern string) (string, func(), error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", nil, fmt.Errorf("create temp file: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", nil, fmt.Errorf("close temp file: %w", err)
	}
	return path, func() { _ = os.Remove(path) }, nil
}
