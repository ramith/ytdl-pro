package ytdlpro

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const localLlamaHeaderPath = "internal/ytdlpro/metadata/model/llama/llama.h"

func runSetup(ctx context.Context, cfg Config) error {
	fmt.Println("setup: preparing embedded libllama runtime")

	if !cfg.Setup.SkipRuntime {
		headerPath, libPath, err := ensureLocalLlamaRuntime(ctx, cfg.Setup.Force)
		if err != nil {
			return err
		}
		fmt.Printf("setup: runtime linked\n  header=%s\n  library=%s\n", headerPath, libPath)
	}

	if !cfg.Setup.SkipModel {
		modelPath, err := ensureModelFile(ctx, cfg.Setup)
		if err != nil {
			return err
		}
		fmt.Printf("setup: model ready\n  model=%s\n", modelPath)
	}

	if !cfg.Setup.SkipBuild {
		if err := buildTaggedBinary(ctx); err != nil {
			return err
		}
		fmt.Println("setup: built ./bin/ytdl-pro with -tags libllama")
	}

	fmt.Println("setup: complete")
	return nil
}

func ensureLocalLlamaRuntime(ctx context.Context, force bool) (string, string, error) {
	if runtime.GOOS == "darwin" {
		if err := ensureHomebrewLlama(ctx); err != nil {
			return "", "", err
		}
	}

	headerPath, libPath, err := detectLlamaArtifacts(ctx)
	if err != nil {
		return "", "", err
	}

	localHeaderAbs, err := filepath.Abs(localLlamaHeaderPath)
	if err != nil {
		return "", "", fmt.Errorf("resolve local llama header path: %w", err)
	}
	localLibName := filepath.Base(libPath)
	localLibAbs, err := filepath.Abs(filepath.Join("lib", localLibName))
	if err != nil {
		return "", "", fmt.Errorf("resolve local llama library path: %w", err)
	}

	if err := ensureSymlink(headerPath, localHeaderAbs, force); err != nil {
		return "", "", err
	}
	if err := ensureSymlink(libPath, localLibAbs, force); err != nil {
		return "", "", err
	}

	return localHeaderAbs, localLibAbs, nil
}

func ensureHomebrewLlama(ctx context.Context) error {
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		return fmt.Errorf("Homebrew not found; install Homebrew or provide libllama manually")
	}

	check := exec.CommandContext(ctx, brewPath, "list", "llama.cpp")
	if err := check.Run(); err == nil {
		return nil
	}

	fmt.Println("setup: installing llama.cpp with Homebrew")
	install := exec.CommandContext(ctx, brewPath, "install", "llama.cpp")
	install.Stdout = os.Stdout
	install.Stderr = os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("install llama.cpp with Homebrew: %w", err)
	}
	return nil
}

func detectLlamaArtifacts(ctx context.Context) (string, string, error) {
	libName := "libllama.so"
	if runtime.GOOS == "darwin" {
		libName = "libllama.dylib"
	}

	headerCandidates := []string{
		"/opt/homebrew/include/llama.h",
		"/usr/local/include/llama.h",
		"/usr/include/llama.h",
	}
	libCandidates := []string{
		filepath.Join("/opt/homebrew/lib", libName),
		filepath.Join("/usr/local/lib", libName),
		filepath.Join("/usr/lib", libName),
	}

	if prefix, err := brewPrefix(ctx, "llama.cpp"); err == nil {
		headerCandidates = append([]string{filepath.Join(prefix, "include", "llama.h")}, headerCandidates...)
		libCandidates = append([]string{filepath.Join(prefix, "lib", libName)}, libCandidates...)
	}

	headerPath, err := firstExistingFile(headerCandidates...)
	if err != nil {
		return "", "", fmt.Errorf("find llama header: %w", err)
	}
	libPath, err := firstExistingFile(libCandidates...)
	if err != nil {
		return "", "", fmt.Errorf("find libllama library: %w", err)
	}
	return headerPath, libPath, nil
}

func brewPrefix(ctx context.Context, formula string) (string, error) {
	brewPath, err := exec.LookPath("brew")
	if err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, brewPath, "--prefix", formula)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func firstExistingFile(paths ...string) (string, error) {
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		info, err := os.Stat(path)
		if err == nil && !info.IsDir() {
			return path, nil
		}
	}
	return "", errors.New("no matching file found")
}

func ensureSymlink(source, target string, force bool) error {
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("create directory for %s: %w", target, err)
	}

	existing, err := os.Lstat(target)
	if err == nil {
		if !force {
			if existing.Mode()&os.ModeSymlink != 0 {
				if resolved, linkErr := os.Readlink(target); linkErr == nil && resolved == source {
					return nil
				}
			}
			return fmt.Errorf("path already exists: %s (use --force to replace)", target)
		}
		if removeErr := os.Remove(target); removeErr != nil {
			return fmt.Errorf("remove existing path %s: %w", target, removeErr)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect existing path %s: %w", target, err)
	}

	if err := os.Symlink(source, target); err != nil {
		return fmt.Errorf("create symlink %s -> %s: %w", target, source, err)
	}
	return nil
}

func ensureModelFile(ctx context.Context, cfg SetupConfig) (string, error) {
	modelPath := strings.TrimSpace(cfg.ModelPath)
	if modelPath == "" {
		return "", fmt.Errorf("missing model path")
	}
	if err := os.MkdirAll(filepath.Dir(modelPath), 0755); err != nil {
		return "", fmt.Errorf("create model directory: %w", err)
	}

	if info, err := os.Stat(modelPath); err == nil && info.Size() > 0 && !cfg.Force {
		return modelPath, nil
	}

	tmpPath := modelPath + ".part"
	if err := os.RemoveAll(tmpPath); err != nil {
		return "", fmt.Errorf("clear partial model file: %w", err)
	}

	fmt.Printf("setup: downloading model\n  url=%s\n", cfg.ModelURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.ModelURL, nil)
	if err != nil {
		return "", fmt.Errorf("create model download request: %w", err)
	}

	client := &http.Client{Timeout: 0}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("download model: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("download model returned %s", resp.Status)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create partial model file: %w", err)
	}
	defer out.Close()

	written, err := io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("write model file: %w", err)
	}
	if written == 0 {
		return "", fmt.Errorf("downloaded empty model file")
	}
	if err := out.Close(); err != nil {
		return "", fmt.Errorf("close model file: %w", err)
	}
	if err := os.Rename(tmpPath, modelPath); err != nil {
		return "", fmt.Errorf("finalize model file: %w", err)
	}
	return modelPath, nil
}

func buildTaggedBinary(ctx context.Context) error {
	if err := os.MkdirAll("bin", 0755); err != nil {
		return fmt.Errorf("create bin directory: %w", err)
	}
	fmt.Println("setup: building tagged binary")
	cmd := exec.CommandContext(ctx, "go", "build", "-tags", "libllama", "-o", "./bin/ytdl-pro", "./cmd/ytdl-pro")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=1",
	)
	return cmd.Run()
}
