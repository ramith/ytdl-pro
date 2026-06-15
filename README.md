# ytdl-pro

`ytdl-pro` is a command-line YouTube downloader written in Go. It can list
available formats, download video or audio, merge separate video and audio
streams, and convert audio to MP3, FLAC, WAV, or ALAC.

Only download videos that you own, license, or have permission to download.
Downloads require the explicit `-i-have-rights` flag.

## Requirements

- Go 1.26 or newer
- FFmpeg for video/audio merging and audio conversion

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

## Usage

Show all options:

```sh
./bin/ytdl-pro -h
```

List the formats available for a video:

```sh
./bin/ytdl-pro -url "https://www.youtube.com/watch?v=VIDEO_ID" -list
```

Download the best available video:

```sh
./bin/ytdl-pro \
  -url "https://www.youtube.com/watch?v=VIDEO_ID" \
  -i-have-rights
```

Download a specific video quality:

```sh
./bin/ytdl-pro \
  -url "https://www.youtube.com/watch?v=VIDEO_ID" \
  -quality 1080p \
  -out ./downloads \
  -i-have-rights
```

Quality may be `best`, a resolution such as `720p`, a YouTube quality label
such as `hd1080`, or an itag shown by `-list`.

Download the best original audio stream:

```sh
./bin/ytdl-pro \
  -url "https://www.youtube.com/watch?v=VIDEO_ID" \
  -audio-only \
  -i-have-rights
```

Download and convert audio to VBR MP3:

```sh
./bin/ytdl-pro \
  -url "https://www.youtube.com/watch?v=VIDEO_ID" \
  -audio-only \
  -audio-format mp3 \
  -mp3-mode vbr \
  -mp3-vbr 0 \
  -i-have-rights
```

Download and convert audio to a 192 kbps MP3:

```sh
./bin/ytdl-pro \
  -url "https://www.youtube.com/watch?v=VIDEO_ID" \
  -audio-only \
  -audio-format mp3 \
  -mp3-mode bitrate \
  -mp3-bitrate 192k \
  -i-have-rights
```

Other audio output formats are `flac`, `wav`, and `alac`. Source audio quality
may be `best`, `high`, `medium`, `low`, a bitrate such as `128k`, or an itag.

Useful output options:

```text
-out ./downloads       Set the output directory
-filename example.mp4  Set the output filename
-overwrite             Replace an existing output file
-timeout 1h            Set the overall operation timeout
-timeout 0             Disable the timeout
```

## Make Targets

```sh
make help
make build
make test
make fmt
make tidy
make clean
```

Pass application arguments through the `run` target:

```sh
make run ARGS='-url "VIDEO_ID" -list'
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

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE).
