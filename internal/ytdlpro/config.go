package ytdlpro

import (
	"errors"
	"flag"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type AudioFormat string

const (
	AudioOriginal AudioFormat = "original"
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
	URL          string
	OutDir       string
	Filename     string
	Overwrite    bool
	Timeout      time.Duration
	ListFormats  bool
	RightsOK     bool
	VideoQuality string
	AudioOnly    bool
	AudioQuality string
	AudioFormat  AudioFormat
	MP3Mode      MP3Mode
	MP3VBR       int
	MP3Bitrate   string
}

func ParseConfig(args []string) (Config, error) {
	var cfg Config

	fs := flag.NewFlagSet("ytdl-pro", flag.ContinueOnError)
	fs.StringVar(&cfg.URL, "url", "", "YouTube video URL or video ID")
	fs.StringVar(&cfg.OutDir, "out", ".", "output directory")
	fs.StringVar(&cfg.Filename, "filename", "", "optional output filename")
	fs.BoolVar(&cfg.Overwrite, "overwrite", false, "overwrite existing file")
	fs.DurationVar(&cfg.Timeout, "timeout", 30*time.Minute, "overall timeout, e.g. 10m, 1h, 0 disables timeout")
	fs.BoolVar(&cfg.ListFormats, "list", false, "list available formats and exit")
	fs.BoolVar(&cfg.RightsOK, "i-have-rights", false, "confirm you own, license, or have permission")
	fs.StringVar(&cfg.VideoQuality, "quality", "best", "video quality: best, 360p, 720p, 1080p, hd720, hd1080, or itag")
	fs.BoolVar(&cfg.AudioOnly, "audio-only", false, "download audio only")
	fs.StringVar(&cfg.AudioQuality, "audio-quality", "best", "source audio quality: best, high, medium, low, 64k, 128k, 192k, or itag")
	fs.Var((*audioFormatValue)(&cfg.AudioFormat), "audio-format", "audio output: original, mp3, flac, lossless, wav, or alac")
	fs.Var((*mp3ModeValue)(&cfg.MP3Mode), "mp3-mode", "MP3 mode: vbr or bitrate")
	fs.IntVar(&cfg.MP3VBR, "mp3-vbr", 0, "MP3 VBR quality: 0 best/largest, 9 lowest/smallest")
	fs.StringVar(&cfg.MP3Bitrate, "mp3-bitrate", "192k", "MP3 bitrate for -mp3-mode bitrate, e.g. 128k, 192k, 320k")

	cfg.AudioFormat = AudioOriginal
	cfg.MP3Mode = MP3VBR

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.URL) == "" {
		return errors.New("missing -url")
	}

	if !c.ListFormats && !c.RightsOK {
		return errors.New("refusing download: pass -i-have-rights only for owned, licensed, or permitted videos")
	}

	if c.Timeout < 0 {
		return errors.New("-timeout cannot be negative")
	}

	if c.AudioFormat == "" {
		return errors.New("missing -audio-format")
	}

	if c.MP3Mode == "" {
		return errors.New("missing -mp3-mode")
	}

	if c.AudioFormat != AudioMP3 && (c.MP3Mode != MP3VBR || c.MP3VBR != 0 || c.MP3Bitrate != "192k") {
		return errors.New("MP3 options require -audio-format mp3")
	}

	if c.AudioFormat == AudioMP3 {
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

	return nil
}

type audioFormatValue AudioFormat

func (v *audioFormatValue) String() string { return string(*v) }

func (v *audioFormatValue) Set(raw string) error {
	value := strings.ToLower(strings.TrimSpace(raw))

	switch value {
	case "", "original":
		*v = audioFormatValue(AudioOriginal)
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
