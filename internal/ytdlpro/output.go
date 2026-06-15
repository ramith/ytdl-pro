package ytdlpro

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/kkdai/youtube/v2"
)

func PrintFormats(w io.Writer, video *youtube.Video) {
	fmt.Fprintln(w, "Title:", video.Title)
	fmt.Fprintln(w, "Author:", video.Author)
	fmt.Fprintln(w)

	fmt.Fprintf(w, "%-12s %-6s %-10s %-8s %-5s %-8s %-10s %-9s %s\n",
		"type", "itag", "quality", "height", "fps", "audioCh", "bitrate", "sizeMB", "mime")

	formats := append(youtube.FormatList(nil), video.Formats...)
	sort.SliceStable(formats, func(i, j int) bool {
		if FormatKind(formats[i]) != FormatKind(formats[j]) {
			return FormatKind(formats[i]) < FormatKind(formats[j])
		}
		if formats[i].Height != formats[j].Height {
			return formats[i].Height > formats[j].Height
		}
		return AudioBitrate(formats[i]) > AudioBitrate(formats[j])
	})

	for _, f := range formats {
		quality := f.QualityLabel
		if quality == "" {
			quality = f.Quality
		}

		audioChannels := "-"
		if f.AudioChannels > 0 {
			audioChannels = strconv.Itoa(f.AudioChannels)
		}

		sizeMB := "-"
		if f.ContentLength > 0 {
			sizeMB = fmt.Sprintf("%.1f", float64(f.ContentLength)/(1024*1024))
		}

		fmt.Fprintf(w, "%-12s %-6d %-10s %-8d %-5d %-8s %-10d %-9s %s\n",
			FormatKind(f), f.ItagNo, quality, f.Height, f.FPS, audioChannels, AudioBitrate(f), sizeMB, f.MimeType)
	}
}

func PrintPlaylist(w io.Writer, playlist *youtube.Playlist) {
	fmt.Fprintln(w, "Playlist:", playlist.Title)
	fmt.Fprintln(w, "Author:", playlist.Author)
	fmt.Fprintf(w, "Items: %d\n\n", len(playlist.Videos))

	for i, video := range playlist.Videos {
		fmt.Fprintf(w, "%4d  %s  %s\n", i+1, video.ID, video.Title)
	}
}

func PrintSelectedAudio(format *youtube.Format, output string) {
	fmt.Println("selected audio:")
	fmt.Printf("  itag=%d\n", format.ItagNo)
	fmt.Printf("  output=%s\n", output)
	fmt.Printf("  mime=%s\n", format.MimeType)
	fmt.Printf("  source bitrate=%d\n", AudioBitrate(*format))
	fmt.Printf("  source audio quality=%s\n", format.AudioQuality)
}

func PrintSelectedVideo(format *youtube.Format, merge bool) {
	fmt.Println("selected video:")
	fmt.Printf("  itag=%d\n", format.ItagNo)
	fmt.Printf("  quality=%s\n", DisplayQuality(format))
	fmt.Printf("  height=%d\n", format.Height)
	fmt.Printf("  fps=%d\n", format.FPS)
	fmt.Printf("  mime=%s\n", format.MimeType)
	fmt.Printf("  merge required=%t\n", merge)
}

func FormatKind(f youtube.Format) string {
	switch {
	case IsVideo(&f) && HasAudio(&f):
		return "video+audio"
	case IsVideo(&f):
		return "video"
	case IsAudio(&f):
		return "audio"
	default:
		return "other"
	}
}

func DisplayQuality(f *youtube.Format) string {
	if f.QualityLabel != "" {
		return f.QualityLabel
	}
	return f.Quality
}

func AudioFormatExtension(format AudioFormat) string {
	switch format {
	case AudioMP3:
		return ".mp3"
	case AudioFLAC:
		return ".flac"
	case AudioWAV:
		return ".wav"
	case AudioALAC:
		return ".m4a"
	default:
		return ".audio"
	}
}

func ExtensionForMIME(mimeType string, fallback string) string {
	mimeType = strings.ToLower(mimeType)
	switch {
	case strings.Contains(mimeType, "audio/mp4"):
		return ".m4a"
	case strings.Contains(mimeType, "video/mp4"):
		return ".mp4"
	case strings.Contains(mimeType, "webm"):
		return ".webm"
	case strings.Contains(mimeType, "opus"):
		return ".opus"
	default:
		return fallback
	}
}
