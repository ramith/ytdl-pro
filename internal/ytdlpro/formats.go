package ytdlpro

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kkdai/youtube/v2"
)

type VideoSelection struct {
	VideoFormat *youtube.Format
	AudioFormat *youtube.Format
	Merge       bool
	OutputExt   string
}

func SelectVideoFormat(video *youtube.Video, quality string) (*VideoSelection, error) {
	matches := FilterVideoFormats(video.Formats, quality)
	if len(matches) == 0 {
		return nil, fmt.Errorf("no video format found for quality %q", quality)
	}

	SortVideoFormats(matches)

	for i := range matches {
		if IsVideo(&matches[i]) && HasAudio(&matches[i]) {
			return &VideoSelection{VideoFormat: &matches[i], OutputExt: ExtensionForMIME(matches[i].MimeType, ".mp4")}, nil
		}
	}

	videoOnly := BestVideoOnly(matches)
	if videoOnly == nil {
		return nil, errors.New("no video-only format found")
	}

	audio := BestAudioForVideo(video.Formats, videoOnly)
	if audio == nil {
		return nil, errors.New("no audio format found for video merge")
	}

	ext := ".mkv"
	if IsMP4(videoOnly.MimeType) && IsMP4(audio.MimeType) {
		ext = ".mp4"
	}

	return &VideoSelection{VideoFormat: videoOnly, AudioFormat: audio, Merge: true, OutputExt: ext}, nil
}

func FilterVideoFormats(formats youtube.FormatList, quality string) youtube.FormatList {
	quality = NormalizeQuality(quality)

	if quality == "" || quality == "best" {
		return formats.Select(func(f youtube.Format) bool { return IsVideo(&f) })
	}

	itag, hasItag := ParseInt(quality)
	height := ParseHeight(quality)

	return formats.Select(func(f youtube.Format) bool {
		if !IsVideo(&f) {
			return false
		}
		if hasItag && f.ItagNo == itag {
			return true
		}

		q := strings.ToLower(f.Quality)
		label := strings.ToLower(f.QualityLabel)
		if q == quality || label == quality {
			return true
		}
		if height > 0 && f.Height == height {
			return true
		}
		if height > 0 && q == "hd"+strconv.Itoa(height) {
			return true
		}

		return false
	})
}

func SelectAudioFormat(video *youtube.Video, quality string) (*youtube.Format, error) {
	audio := video.Formats.Select(func(f youtube.Format) bool { return IsAudio(&f) })
	if len(audio) == 0 {
		return nil, errors.New("no audio formats found")
	}

	quality = strings.ToLower(strings.TrimSpace(quality))
	if quality == "" || quality == "best" {
		return BestAudioByBitrate(audio), nil
	}

	if itag, hasItag := ParseInt(quality); hasItag {
		matches := audio.Itag(itag)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no audio format found for itag %d", itag)
		}
		return &matches[0], nil
	}

	switch quality {
	case "high":
		return ClosestAudioByBitrate(audio, 192000), nil
	case "medium":
		return ClosestAudioByBitrate(audio, 128000), nil
	case "low":
		return ClosestAudioByBitrate(audio, 64000), nil
	}

	if kbps, ok := ParseBitrateKbps(quality); ok {
		return ClosestAudioByBitrate(audio, kbps*1000), nil
	}

	matches := audio.Select(func(f youtube.Format) bool {
		return strings.EqualFold(f.AudioQuality, quality)
	})
	if len(matches) > 0 {
		return BestAudioByBitrate(matches), nil
	}

	return nil, fmt.Errorf("unsupported audio quality %q", quality)
}

func BestVideoOnly(formats youtube.FormatList) *youtube.Format {
	matches := formats.Select(func(f youtube.Format) bool { return IsVideo(&f) && !HasAudio(&f) })
	if len(matches) == 0 {
		return nil
	}
	SortVideoFormats(matches)
	return &matches[0]
}

func BestAudioForVideo(formats youtube.FormatList, videoFormat *youtube.Format) *youtube.Format {
	audio := formats.Select(func(f youtube.Format) bool { return IsAudio(&f) })
	if len(audio) == 0 {
		return nil
	}

	if IsMP4(videoFormat.MimeType) {
		mp4Audio := audio.Select(func(f youtube.Format) bool { return IsMP4(f.MimeType) })
		if len(mp4Audio) > 0 {
			return BestAudioByBitrate(mp4Audio)
		}
	}

	return BestAudioByBitrate(audio)
}

func BestAudioByBitrate(formats youtube.FormatList) *youtube.Format {
	if len(formats) == 0 {
		return nil
	}

	bestIdx := 0
	bestBitrate := -1
	for i := range formats {
		if br := AudioBitrate(formats[i]); br > bestBitrate {
			bestIdx = i
			bestBitrate = br
		}
	}
	return &formats[bestIdx]
}

func ClosestAudioByBitrate(formats youtube.FormatList, target int) *youtube.Format {
	if len(formats) == 0 {
		return nil
	}

	bestIdx := 0
	bestScore := int(^uint(0) >> 1)

	for i := range formats {
		br := AudioBitrate(formats[i])
		if br <= 0 {
			continue
		}

		score := Abs(br - target)
		if br < target {
			score += target
		}

		if score < bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return &formats[bestIdx]
}

func SortVideoFormats(formats youtube.FormatList) {
	sort.SliceStable(formats, func(i, j int) bool {
		if formats[i].Height != formats[j].Height {
			return formats[i].Height > formats[j].Height
		}
		if formats[i].FPS != formats[j].FPS {
			return formats[i].FPS > formats[j].FPS
		}
		return formats[i].Bitrate > formats[j].Bitrate
	})
}

func AudioBitrate(f youtube.Format) int {
	if f.AverageBitrate > 0 {
		return f.AverageBitrate
	}
	return f.Bitrate
}

func NormalizeQuality(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, " ", "")
	switch value {
	case "1080", "720", "480", "360", "240", "144":
		return value + "p"
	default:
		return value
	}
}

func ParseHeight(value string) int {
	re := regexp.MustCompile(`(\d{3,4})p?`)
	match := re.FindStringSubmatch(value)
	if len(match) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(match[1])
	return n
}

func ParseInt(value string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return n, true
}

func IsVideo(f *youtube.Format) bool  { return strings.Contains(strings.ToLower(f.MimeType), "video") }
func IsAudio(f *youtube.Format) bool  { return strings.Contains(strings.ToLower(f.MimeType), "audio") }
func HasAudio(f *youtube.Format) bool { return f.AudioChannels > 0 }
func IsMP4(mime string) bool          { return strings.Contains(strings.ToLower(mime), "mp4") }
func Abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
