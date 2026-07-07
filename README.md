# ytdl-pro

`ytdl-pro` is a command-line YouTube downloader written in Go. It can download
single videos or complete playlists, list available formats, merge separate
video and audio streams, and save audio as original source, smart auto-selected
audio, MP3, FLAC, WAV, or ALAC.

Only download videos that you own, license, or have permission to download.

## Requirements

- Go 1.26 or newer
- FFmpeg for video/audio merging and audio conversion
- `ffprobe`, which is included with standard FFmpeg installs, for metadata
  verification

On macOS, FFmpeg can be installed with Homebrew:

```sh
brew install ffmpeg
```

## Build

Download dependencies and build the executable:

```sh
go mod download
make build
```

The executable is created at `./bin/ytdl-pro`.

You can also build without Make:

```sh
go build -o bin/ytdl-pro ./cmd/ytdl-pro
```

Install it into your configured `GOBIN` or `GOPATH/bin`:

```sh
make install
```

Install zsh tab completion:

```sh
make install-completion
```

Your zsh configuration must include the completion directory in `fpath` and
initialize completions:

```sh
fpath=(/opt/homebrew/share/zsh/site-functions $fpath)
autoload -Uz compinit
compinit
```

## Usage

For the shortest path, provide only the URL or video ID and answer the
questions:

```sh
./bin/ytdl-pro "https://www.youtube.com/watch?v=VIDEO_ID"
```

For explicit non-interactive usage, use the `download` command:

```sh
./bin/ytdl-pro download "https://www.youtube.com/watch?v=VIDEO_ID"
```

The wizard asks whether to download video or audio, which quality and output
format to use, and where to save the file.
The filename comes from the video title. If that filename already exists, a
numbered suffix such as ` (1)` or ` (2)` is added automatically. Press Enter
to accept each default.

### Playlists

Playlist URLs are detected automatically. Choose options once in the
interactive wizard and they are applied to every playlist item:

```sh
ytdl-pro download "https://www.youtube.com/playlist?list=PLAYLIST_ID"
```

`music.youtube.com` playlist links are also supported and handled the same way:

```sh
ytdl-pro download "https://music.youtube.com/playlist?list=PLAYLIST_ID"
```

For non-interactive playlist downloads, use the same switches as a single
video:

```sh
ytdl-pro download \
  "https://www.youtube.com/playlist?list=PLAYLIST_ID" \
  -audio-only \
  -audio-format mp3 \
  -mp3-mode vbr \
  -mp3-vbr 0 \
  -out ./playlist-downloads
```

Normal playlist URLs and IDs are detected automatically. Use `-playlist` to
force playlist handling for an unusual URL or ID:

```sh
ytdl-pro download "PLAYLIST_ID" -playlist -list
```

YouTube radio/mix links (`list=RD...`) download the currently selected video
instead of being treated as a playlist because YouTube does not expose those
mixes through the playlist API.

Playlist items are downloaded sequentially. Unavailable items are reported
and skipped so the remaining items can continue. Each filename comes from its
video title, with a numbered suffix added when needed. The `-filename` option
cannot be used for playlists. The `-timeout` value applies separately to each
playlist item rather than to the entire playlist.

Show all options:

```sh
./bin/ytdl-pro -h
```

Most command-line flags already have safe defaults, so for common cases you can
usually provide just the URL and optionally `-audio-only`.

List the formats available for a video:

```sh
./bin/ytdl-pro download "https://www.youtube.com/watch?v=VIDEO_ID" -list
```

Download the best available video:

```sh
./bin/ytdl-pro download \
  "https://www.youtube.com/watch?v=VIDEO_ID"
```

Download a specific video quality:

```sh
./bin/ytdl-pro download \
  "https://www.youtube.com/watch?v=VIDEO_ID" \
  -quality 1080p \
  -out ./downloads
```

Quality may be `best`, a resolution such as `720p`, a YouTube quality label
such as `hd1080`, or an itag shown by `-list`.

Download the best original audio stream:

```sh
./bin/ytdl-pro download \
  "https://www.youtube.com/watch?v=VIDEO_ID" \
  -audio-only
```

Download and convert audio to VBR MP3:

```sh
./bin/ytdl-pro download \
  "https://www.youtube.com/watch?v=VIDEO_ID" \
  -audio-only \
  -audio-format mp3 \
  -mp3-mode vbr \
  -mp3-vbr 0
```

Download and convert audio to a 192 kbps MP3:

```sh
./bin/ytdl-pro download \
  "https://www.youtube.com/watch?v=VIDEO_ID" \
  -audio-only \
  -audio-format mp3 \
  -mp3-mode bitrate \
  -mp3-bitrate 192k
```

Use `-audio-format smart` to prefer a lossless M4A source when YouTube exposes
one, otherwise fall back to the highest-quality source and transcode it using
your MP3 settings:

```sh
./bin/ytdl-pro download \
  "https://www.youtube.com/watch?v=VIDEO_ID" \
  -audio-only \
  -audio-format smart \
  -mp3-mode vbr \
  -mp3-vbr 0
```

Other audio output formats are `flac`, `wav`, and `alac`. Source audio quality
may be `best`, `high`, `medium`, `low`, a bitrate such as `128k`, or an itag.

Useful output options:

```text
-out ./downloads       Set the output directory
-filename example.mp4  Set an explicit output filename
-overwrite             Replace an existing output file
-timeout 1h            Set the operation timeout; per item for playlists
-timeout 0             Disable the timeout
```

Default non-interactive behavior:

```text
-quality best
-audio-quality best
-audio-format original
-mp3-mode vbr
-mp3-vbr 0
-mp3-bitrate 192k
-timeout 30m
-out .
```

### Metadata Enrichment

`ytdl-pro enrich` can enrich tags for a YouTube track, a local audio file, or a
directory of existing audio files. Normal use does not require any separate
model server or daemon process.

Simple examples:

```sh
ytdl-pro enrich "https://youtube.com/watch?v=VIDEO_ID"
ytdl-pro enrich ./song.mp3
ytdl-pro enrich ./Music --recursive
```

What the command does:

- Uses the download metadata, existing tags, and structured candidate lookup.
- Writes only source-backed fields that pass the confidence thresholds.
- Falls back to deterministic scoring if the local model runtime is unavailable.
- Continues processing the rest of a directory or playlist even if one file fails.

Status labels:

```text
enriched
partially enriched
base tagged
skipped
failed
```

Useful options:

```text
--recursive                       Recurse into nested directories when enriching a folder
--dry-run                         Analyze without writing tags
--review                          Mark likely matches for review instead of writing
--write-base-tags                 Write base source tags when enrichment is skipped
--json-report report.json         Write a JSON report for the run
--explain                         Show why a file was or was not changed
--debug                           Show internal diagnostics
```

When metadata writing is enabled, tag rewriting uses `ffmpeg` stream copy and
keeps the audio payload intact. The rewritten file is verified with `ffprobe`
before replacing the original. A backup file such as `track.mp3.bak` is kept
unless `--no-backup` is provided.

## Make Targets

```sh
make help
make build
make test
make fmt
make tidy
make install-completion
make clean
```

Pass application arguments through the `run` target:

```sh
make run ARGS='"VIDEO_ID"'
```

Commands with additional flags remain non-interactive:

```sh
./bin/ytdl-pro \
  -url "https://www.youtube.com/watch?v=VIDEO_ID" \
  -quality 1080p
```

## Development

Project layout:

```text
.
├── cmd/ytdl-pro/       CLI entrypoint
├── internal/ytdlpro/   Application, download, format, and file logic
├── Makefile
├── README.md
├── go.mod
└── go.sum
```

Format, test, and build the project:

```sh
make fmt
make test
make build
```

Build with the embedded local runtime:

```sh
go build -tags libllama -o bin/ytdl-pro ./cmd/ytdl-pro
```

Bootstrap the native runtime, local model, and tagged binary in one step:

```sh
go run ./cmd/ytdl-pro setup
```

The tagged build expects:

- `libllama` to be available either in `./lib` or on the system library path
- a local GGUF model file at `./models/qwen3-1.7b-instruct-q4_k_m.gguf`, or an
  override passed through `-metadata-model-path`

Without the `libllama` build tag, the binary still builds and runs using
deterministic metadata scoring only.

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
