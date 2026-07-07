package ytdlpro

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type AudioFormat string

const (
	AudioOriginal AudioFormat = "original"
	AudioSmart    AudioFormat = "smart"
	AudioMP3      AudioFormat = "mp3"
	AudioFLAC     AudioFormat = "flac"
	AudioWAV      AudioFormat = "wav"
	AudioALAC     AudioFormat = "alac"
)

type MP3Mode string

const (
	MP3VBR     MP3Mode = "vbr"
	MP3Bitrate MP3Mode = "bitrate"
)

type Config struct {
	Command      string
	URL          string
	Interactive  bool
	Debug        bool
	Explain      bool
	OutDir       string
	Filename     string
	Overwrite    bool
	Timeout      time.Duration
	ListFormats  bool
	Playlist     bool
	RightsOK     bool
	VideoQuality string
	AudioOnly    bool
	AudioQuality string
	AudioFormat  AudioFormat
	MP3Mode      MP3Mode
	MP3VBR       int
	MP3Bitrate   string
	Metadata     MetadataConfig
	Setup        SetupConfig
}

func DefaultConfig() Config {
	return Config{
		Command:      "download",
		OutDir:       ".",
		Timeout:      30 * time.Minute,
		VideoQuality: "best",
		AudioQuality: "best",
		AudioFormat:  AudioOriginal,
		MP3Mode:      MP3VBR,
		MP3VBR:       0,
		MP3Bitrate:   "192k",
		Metadata:     DefaultMetadataConfig(),
		Setup:        DefaultSetupConfig(),
	}
}

func ParseConfig(args []string) (Config, error) {
	cfg := DefaultConfig()
	if len(args) == 0 {
		return Config{}, errors.New("missing input; use `ytdl-pro download URL` or `ytdl-pro enrich PATH`")
	}

	switch args[0] {
	case "-h", "--help", "help":
		printRootUsage(os.Stderr)
		return Config{}, flag.ErrHelp
	case "download":
		return parseDownloadConfig(cfg, args[1:], true)
	case "enrich":
		return parseEnrichConfig(cfg, args[1:])
	case "setup":
		return parseSetupConfig(cfg, args[1:])
	default:
		return parseDownloadConfig(cfg, args, false)
	}
}

func (c Config) Validate() error {
	hasURL := strings.TrimSpace(c.URL) != ""
	hasMetadataPath := strings.TrimSpace(c.Metadata.Path) != ""

	if !hasURL && !hasMetadataPath {
		return errors.New("missing input; use `ytdl-pro download URL` or `ytdl-pro enrich PATH`")
	}
	if hasURL && hasMetadataPath {
		return errors.New("cannot combine a download URL with a local enrich path")
	}

	if c.Timeout < 0 {
		return errors.New("-timeout cannot be negative")
	}

	if c.Playlist && strings.TrimSpace(c.Filename) != "" {
		return errors.New("-filename cannot be used with a playlist; filenames come from each video title")
	}
	if hasMetadataPath {
		if c.Playlist {
			return errors.New("-playlist cannot be used with a local enrich path")
		}
		if strings.TrimSpace(c.Filename) != "" {
			return errors.New("-filename cannot be used with a local enrich path")
		}
	}

	if c.AudioFormat == "" {
		return errors.New("missing -audio-format")
	}

	if c.MP3Mode == "" {
		return errors.New("missing -mp3-mode")
	}

	if c.AudioFormat != AudioMP3 && c.AudioFormat != AudioSmart && (c.MP3Mode != MP3VBR || c.MP3VBR != 0 || c.MP3Bitrate != "192k") {
		return errors.New("MP3 options require -audio-format mp3 or smart")
	}

	if c.AudioFormat == AudioMP3 || c.AudioFormat == AudioSmart {
		if c.MP3Mode == MP3VBR {
			if c.MP3VBR < 0 || c.MP3VBR > 9 {
				return errors.New("-mp3-vbr must be between 0 and 9")
			}
		}

		if c.MP3Mode == MP3Bitrate {
			if _, err := NormalizeBitrate(c.MP3Bitrate); err != nil {
				return err
			}
		}
	}

	if err := c.Metadata.Validate(); err != nil {
		return err
	}

	return nil
}

func parseDownloadConfig(cfg Config, args []string, explicitCommand bool) (Config, error) {
	cfg.Command = "download"

	fs := flag.NewFlagSet("download", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		printDownloadUsage(fs.Output())
		fmt.Fprintln(fs.Output())
		fs.PrintDefaults()
	}

	fs.StringVar(&cfg.URL, "url", cfg.URL, "YouTube video or playlist URL/ID")
	fs.StringVar(&cfg.OutDir, "out", cfg.OutDir, "output directory")
	fs.StringVar(&cfg.Filename, "filename", "", "optional output filename")
	fs.BoolVar(&cfg.Overwrite, "overwrite", cfg.Overwrite, "overwrite existing file")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "operation timeout; applied per playlist item, e.g. 10m, 1h, 0 disables timeout")
	fs.BoolVar(&cfg.ListFormats, "list", cfg.ListFormats, "list available formats or playlist items and exit")
	fs.BoolVar(&cfg.Playlist, "playlist", cfg.Playlist, "treat the URL or ID as a playlist")
	fs.StringVar(&cfg.VideoQuality, "quality", cfg.VideoQuality, "video quality: best, 360p, 720p, 1080p, hd720, hd1080, or itag")
	fs.BoolVar(&cfg.AudioOnly, "audio-only", cfg.AudioOnly, "download audio only")
	fs.StringVar(&cfg.AudioQuality, "audio-quality", cfg.AudioQuality, "source audio quality: best, high, medium, low, 64k, 128k, 192k, or itag")
	fs.Var((*audioFormatValue)(&cfg.AudioFormat), "audio-format", "audio output: original, smart, mp3, flac, lossless, wav, or alac")
	fs.Var((*mp3ModeValue)(&cfg.MP3Mode), "mp3-mode", "MP3 mode: vbr or bitrate")
	fs.IntVar(&cfg.MP3VBR, "mp3-vbr", cfg.MP3VBR, "MP3 VBR quality: 0 best/largest, 9 lowest/smallest")
	fs.StringVar(&cfg.MP3Bitrate, "mp3-bitrate", cfg.MP3Bitrate, "MP3 bitrate for -mp3-mode bitrate, e.g. 128k, 192k, 320k")

	if err := fs.Parse(normalizeArgs(fs, args)); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 1 {
		return Config{}, errors.New("expected one URL or video ID")
	}
	if fs.NArg() == 1 {
		if strings.TrimSpace(cfg.URL) != "" {
			return Config{}, errors.New("provide the URL either positionally or with -url, not both")
		}
		cfg.URL = fs.Arg(0)
	}

	visited := visitedNames(fs)
	cfg.Interactive = !explicitCommand && len(visited) == 0 && strings.TrimSpace(cfg.URL) != ""
	if explicitCommand {
		cfg.Interactive = false
	}

	if cfg.Interactive {
		return cfg, nil
	}
	return cfg, cfg.Validate()
}

func parseEnrichConfig(cfg Config, args []string) (Config, error) {
	cfg.Command = "enrich"
	cfg.AudioOnly = true
	cfg.Metadata.Enabled = true
	cfg.Metadata.Write = true

	fs := flag.NewFlagSet("enrich", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		printEnrichUsage(fs.Output())
		fmt.Fprintln(fs.Output())
		fs.PrintDefaults()
	}

	fs.StringVar(&cfg.URL, "url", cfg.URL, "YouTube URL to download and enrich")
	fs.StringVar(&cfg.OutDir, "out", cfg.OutDir, "output directory when enriching a YouTube URL")
	fs.StringVar(&cfg.Filename, "filename", "", "optional output filename when enriching a YouTube URL")
	fs.BoolVar(&cfg.Overwrite, "overwrite", cfg.Overwrite, "overwrite an existing downloaded file")
	fs.DurationVar(&cfg.Metadata.Timeout, "timeout", cfg.Metadata.Timeout, "timeout per file")
	fs.BoolVar(&cfg.Metadata.DryRun, "dry-run", false, "analyze without writing tags")
	fs.BoolVar(&cfg.Metadata.ReviewOnly, "review", false, "analyze and mark likely matches for review")
	fs.BoolVar(&cfg.Metadata.Recursive, "recursive", cfg.Metadata.Recursive, "scan directories recursively")
	fs.BoolVar(&cfg.Metadata.WriteBaseTags, "write-base-tags", cfg.Metadata.WriteBaseTags, "write base source tags when enrichment is skipped")
	fs.StringVar(&cfg.Metadata.JSONReport, "json-report", "", "write a machine-readable JSON report")
	fs.BoolVar(&cfg.Metadata.NoBackup, "no-backup", cfg.Metadata.NoBackup, "do not keep a backup before rewriting local file metadata")
	fs.BoolVar(&cfg.Explain, "explain", cfg.Explain, "show why a file was or was not changed")
	fs.BoolVar(&cfg.Debug, "debug", cfg.Debug, "show internal diagnostics")

	if err := fs.Parse(normalizeArgs(fs, args)); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 1 {
		return Config{}, errors.New("expected one YouTube URL or local path")
	}
	if fs.NArg() == 1 {
		arg := fs.Arg(0)
		if info, err := os.Stat(arg); err == nil && info != nil {
			cfg.Metadata.Path = arg
		} else {
			cfg.URL = arg
		}
	}

	cfg.Metadata.Debug = cfg.Debug
	cfg.Metadata.Explain = cfg.Explain
	cfg.Metadata.Enabled = true
	cfg.Metadata.Write = !cfg.Metadata.DryRun && !cfg.Metadata.ReviewOnly
	if !cfg.Metadata.Write {
		cfg.Metadata.DryRun = true
	}
	cfg.Interactive = false

	if strings.TrimSpace(cfg.URL) == "" && strings.TrimSpace(cfg.Metadata.Path) == "" {
		return Config{}, errors.New("missing input; use `ytdl-pro enrich URL` or `ytdl-pro enrich PATH`")
	}
	return cfg, cfg.Validate()
}

func printRootUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ytdl-pro <url>")
	fmt.Fprintln(w, "  ytdl-pro download <url> [options]")
	fmt.Fprintln(w, "  ytdl-pro enrich <url-or-path> [options]")
	fmt.Fprintln(w, "  ytdl-pro setup [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  download   Download video or audio from YouTube")
	fmt.Fprintln(w, "  enrich     Download or process audio and write improved tags")
	fmt.Fprintln(w, "  setup      Install local runtime dependencies and the default model")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Use `ytdl-pro download -h`, `ytdl-pro enrich -h`, or `ytdl-pro setup -h` for command-specific help.")
}

func printDownloadUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ytdl-pro <url>")
	fmt.Fprintln(w, "  ytdl-pro download <url> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  ytdl-pro \"https://www.youtube.com/watch?v=VIDEO_ID\"")
	fmt.Fprintln(w, "  ytdl-pro download \"https://www.youtube.com/watch?v=VIDEO_ID\" -audio-only")
	fmt.Fprintln(w, "  ytdl-pro download \"https://www.youtube.com/playlist?list=PLAYLIST_ID\" -audio-only -audio-format mp3")
}

func printEnrichUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ytdl-pro enrich <url-or-path> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  ytdl-pro enrich \"https://youtube.com/watch?v=VIDEO_ID\"")
	fmt.Fprintln(w, "  ytdl-pro enrich ./song.mp3")
	fmt.Fprintln(w, "  ytdl-pro enrich ./Music --recursive")
}

func parseSetupConfig(cfg Config, args []string) (Config, error) {
	cfg.Command = "setup"

	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() {
		printSetupUsage(fs.Output())
		fmt.Fprintln(fs.Output())
		fs.PrintDefaults()
	}

	fs.BoolVar(&cfg.Setup.SkipRuntime, "skip-runtime", cfg.Setup.SkipRuntime, "skip runtime installation and linking")
	fs.BoolVar(&cfg.Setup.SkipModel, "skip-model", cfg.Setup.SkipModel, "skip model download")
	fs.BoolVar(&cfg.Setup.SkipBuild, "skip-build", cfg.Setup.SkipBuild, "skip building the tagged binary")
	fs.BoolVar(&cfg.Setup.Force, "force", cfg.Setup.Force, "replace existing symlinks, model file, or binary")
	fs.StringVar(&cfg.Setup.ModelURL, "model-url", cfg.Setup.ModelURL, "override the default GGUF model download URL")
	fs.StringVar(&cfg.Setup.ModelPath, "model-path", cfg.Setup.ModelPath, "path to store the GGUF model")

	if err := fs.Parse(normalizeArgs(fs, args)); err != nil {
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, errors.New("setup does not accept positional arguments")
	}
	cfg.Metadata.ModelPath = cfg.Setup.ModelPath
	return cfg, cfg.Setup.Validate()
}

func printSetupUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  ytdl-pro setup [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Examples:")
	fmt.Fprintln(w, "  ytdl-pro setup")
	fmt.Fprintln(w, "  ytdl-pro setup --skip-build")
	fmt.Fprintln(w, "  ytdl-pro setup --model-path ./models/qwen3-1.7b-instruct-q4_k_m.gguf")
}

func visitedNames(fs *flag.FlagSet) map[string]bool {
	visited := make(map[string]bool)
	fs.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})
	return visited
}

type boolFlag interface {
	IsBoolFlag() bool
}

func normalizeArgs(fs *flag.FlagSet, args []string) []string {
	flags := make([]string, 0, len(args))
	positionals := make([]string, 0, 1)

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		flags = append(flags, arg)

		name := strings.TrimLeft(arg, "-")
		if idx := strings.IndexByte(name, '='); idx >= 0 {
			continue
		}
		if flagDef := fs.Lookup(name); flagDef != nil {
			if bf, ok := flagDef.Value.(boolFlag); ok && bf.IsBoolFlag() {
				continue
			}
		}
		if i+1 < len(args) {
			flags = append(flags, args[i+1])
			i++
		}
	}

	return append(flags, positionals...)
}

type audioFormatValue AudioFormat

func (v *audioFormatValue) String() string { return string(*v) }

func (v *audioFormatValue) Set(raw string) error {
	value := strings.ToLower(strings.TrimSpace(raw))

	switch value {
	case "", "original":
		*v = audioFormatValue(AudioOriginal)
	case "smart", "auto":
		*v = audioFormatValue(AudioSmart)
	case "mp3":
		*v = audioFormatValue(AudioMP3)
	case "flac", "lossless":
		*v = audioFormatValue(AudioFLAC)
	case "wav":
		*v = audioFormatValue(AudioWAV)
	case "alac":
		*v = audioFormatValue(AudioALAC)
	default:
		return fmt.Errorf("invalid audio format %q", raw)
	}

	return nil
}

type mp3ModeValue MP3Mode

func (v *mp3ModeValue) String() string { return string(*v) }

func (v *mp3ModeValue) Set(raw string) error {
	value := strings.ToLower(strings.TrimSpace(raw))

	switch value {
	case "vbr":
		*v = mp3ModeValue(MP3VBR)
	case "bitrate", "cbr":
		*v = mp3ModeValue(MP3Bitrate)
	default:
		return fmt.Errorf("invalid MP3 mode %q", raw)
	}

	return nil
}

func NormalizeBitrate(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))

	if value == "" {
		return "", errors.New("empty bitrate")
	}

	if ok, _ := regexp.MatchString(`^[1-9][0-9]*k$`, value); ok {
		return value, nil
	}

	if ok, _ := regexp.MatchString(`^[1-9][0-9]*$`, value); ok {
		return value + "k", nil
	}

	return "", fmt.Errorf("invalid bitrate %q; examples: 128k, 192k, 320k", value)
}

func ParseBitrateKbps(value string) (int, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimSuffix(value, "k")

	kbps, err := strconv.Atoi(value)
	if err != nil || kbps <= 0 {
		return 0, false
	}

	return kbps, true
}
