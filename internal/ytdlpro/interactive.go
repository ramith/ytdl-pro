package ytdlpro

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

func CompleteInteractive(in io.Reader, out io.Writer, cfg Config) (Config, error) {
	if !cfg.Interactive {
		return cfg, nil
	}

	prompt := prompter{
		scanner: bufio.NewScanner(in),
		out:     out,
	}

	fmt.Fprintln(out, "Interactive download setup")
	fmt.Fprintln(out, "Press Enter to accept the value shown in brackets.")
	fmt.Fprintln(out)

	rightsOK, err := prompt.yesNo("Do you own, license, or have permission to download this video?", false)
	if err != nil {
		return Config{}, err
	}
	if !rightsOK {
		return Config{}, errors.New("download cancelled: permission confirmation is required")
	}
	cfg.RightsOK = true

	kind, err := prompt.choice("Download type", "video", "video", "audio")
	if err != nil {
		return Config{}, err
	}
	cfg.AudioOnly = kind == "audio"

	if cfg.AudioOnly {
		cfg.AudioQuality, err = prompt.text("Source audio quality", cfg.AudioQuality)
		if err != nil {
			return Config{}, err
		}

		audioFormat, err := prompt.choice(
			"Audio output format",
			string(cfg.AudioFormat),
			"original", "mp3", "flac", "wav", "alac",
		)
		if err != nil {
			return Config{}, err
		}
		cfg.AudioFormat = AudioFormat(audioFormat)

		if cfg.AudioFormat == AudioMP3 {
			mode, err := prompt.choice("MP3 mode", string(cfg.MP3Mode), "vbr", "bitrate")
			if err != nil {
				return Config{}, err
			}
			cfg.MP3Mode = MP3Mode(mode)

			if cfg.MP3Mode == MP3VBR {
				cfg.MP3VBR, err = prompt.integer("MP3 VBR quality (0 best, 9 smallest)", cfg.MP3VBR, 0, 9)
			} else {
				cfg.MP3Bitrate, err = prompt.validatedText("MP3 bitrate", cfg.MP3Bitrate, func(value string) error {
					_, err := NormalizeBitrate(value)
					return err
				})
			}
			if err != nil {
				return Config{}, err
			}
		}
	} else {
		cfg.VideoQuality, err = prompt.text("Video quality", cfg.VideoQuality)
		if err != nil {
			return Config{}, err
		}
	}

	cfg.OutDir, err = prompt.text("Output directory", cfg.OutDir)
	if err != nil {
		return Config{}, err
	}

	cfg.Interactive = false
	fmt.Fprintln(out)
	return cfg, cfg.Validate()
}

type prompter struct {
	scanner *bufio.Scanner
	out     io.Writer
}

func (p prompter) text(label, defaultValue string) (string, error) {
	fmt.Fprintf(p.out, "%s [%s]: ", label, defaultValue)
	if !p.scanner.Scan() {
		return "", promptReadError(p.scanner.Err())
	}

	value := strings.TrimSpace(p.scanner.Text())
	if value == "" {
		return defaultValue, nil
	}
	return value, nil
}

func (p prompter) validatedText(label, defaultValue string, validate func(string) error) (string, error) {
	for {
		value, err := p.text(label, defaultValue)
		if err != nil {
			return "", err
		}
		if err := validate(value); err != nil {
			fmt.Fprintf(p.out, "Invalid value: %v\n", err)
			continue
		}
		return value, nil
	}
}

func (p prompter) choice(label, defaultValue string, choices ...string) (string, error) {
	allowed := make(map[string]bool, len(choices))
	for _, choice := range choices {
		allowed[choice] = true
	}

	for {
		fmt.Fprintf(p.out, "%s (%s) [%s]: ", label, strings.Join(choices, "/"), defaultValue)
		if !p.scanner.Scan() {
			return "", promptReadError(p.scanner.Err())
		}

		value := strings.ToLower(strings.TrimSpace(p.scanner.Text()))
		if value == "" {
			value = defaultValue
		}
		if allowed[value] {
			return value, nil
		}
		fmt.Fprintf(p.out, "Choose one of: %s\n", strings.Join(choices, ", "))
	}
}

func (p prompter) yesNo(label string, defaultValue bool) (bool, error) {
	defaultLabel := "y"
	if !defaultValue {
		defaultLabel = "n"
	}

	for {
		fmt.Fprintf(p.out, "%s (y/n) [%s]: ", label, defaultLabel)
		if !p.scanner.Scan() {
			return false, promptReadError(p.scanner.Err())
		}

		value := strings.ToLower(strings.TrimSpace(p.scanner.Text()))
		if value == "" {
			return defaultValue, nil
		}
		switch value {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(p.out, "Enter y or n.")
		}
	}
}

func (p prompter) integer(label string, defaultValue, min, max int) (int, error) {
	for {
		value, err := p.text(label, strconv.Itoa(defaultValue))
		if err != nil {
			return 0, err
		}

		number, err := strconv.Atoi(value)
		if err == nil && number >= min && number <= max {
			return number, nil
		}
		fmt.Fprintf(p.out, "Enter a number between %d and %d.\n", min, max)
	}
}

func promptReadError(err error) error {
	if err != nil {
		return fmt.Errorf("read interactive input: %w", err)
	}
	return io.EOF
}
