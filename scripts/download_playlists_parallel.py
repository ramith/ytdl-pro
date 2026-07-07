#!/usr/bin/env python3
"""Parallel YouTube playlist downloader with progress, retries, and tagging.

This helper wraps the existing `ytdl-pro` binary and adds the pieces that are
useful when downloading large playlist sets:

* 5 worker threads by default
* progress output after every completion and at a regular interval
* a download archive so reruns only fetch the delta
* skip detection for files already present in the output directory
* transient error retries with per-item timeout support
* metadata tagging via ffmpeg after each successful download

The script intentionally stays in the Python standard library and uses the
already-installed `ytdl-pro` and `ffmpeg` tools.
"""

from __future__ import annotations

import argparse
import concurrent.futures
import dataclasses
import re
import shutil
import subprocess
import sys
import threading
from pathlib import Path


ITEM_RE = re.compile(r"^\s*\d+\s+([A-Za-z0-9_-]{11})\s+(.+?)\s*$")
PLAYLIST_RE = re.compile(r"^Playlist:\s*(.+?)\s*$")
DOWNLOADED_RE = re.compile(r"^downloaded:\s*(.+?)\s*$", re.IGNORECASE)
SAFE_CHARS_RE = re.compile(r'[\\/:*?"<>|]')
TRANSIENT_PATTERNS = (
    "timed out",
    "timeout",
    "connection reset",
    "connection aborted",
    "temporarily unavailable",
    "429",
    "503",
    "502",
    "network is unreachable",
    "tls: failed to verify certificate",
    "transport endpoint is not connected",
)


@dataclasses.dataclass(frozen=True)
class PlaylistItem:
    video_id: str
    title: str
    album: str


@dataclasses.dataclass
class Result:
    video_id: str
    title: str
    status: str
    message: str = ""
    attempts: int = 0
    path: Path | None = None


@dataclasses.dataclass
class Counters:
    total: int = 0
    skipped: int = 0
    completed: int = 0
    downloaded: int = 0
    tagged: int = 0
    tag_failed: int = 0
    failed: int = 0
    retried: int = 0
    active: int = 0


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Download YouTube playlists in parallel with progress updates.",
    )
    parser.add_argument(
        "playlists",
        nargs="+",
        help="One or more YouTube playlist URLs or playlist IDs",
    )
    parser.add_argument(
        "--out",
        default=str(Path.home() / "english2026"),
        help="Output directory (default: %(default)s)",
    )
    parser.add_argument(
        "--bin",
        default=str(Path(__file__).resolve().parents[1] / "bin" / "ytdl-pro"),
        help="Path to the ytdl-pro binary (default: %(default)s)",
    )
    parser.add_argument(
        "--workers",
        type=int,
        default=5,
        help="Number of parallel download workers (default: %(default)s)",
    )
    parser.add_argument(
        "--progress-interval",
        type=int,
        default=60,
        help="Seconds between regular progress summaries (default: %(default)s)",
    )
    parser.add_argument(
        "--archive",
        default=None,
        help="Archive file to remember downloaded video IDs (default: <out>/.downloaded_ids.txt)",
    )
    parser.add_argument(
        "--quality",
        default="best",
        help="Source audio quality passed to ytdl-pro (default: %(default)s)",
    )
    parser.add_argument(
        "--format",
        default="mp3",
        dest="audio_format",
        help="Audio format passed to ytdl-pro (default: %(default)s)",
    )
    parser.add_argument(
        "--mp3-mode",
        default="vbr",
        choices=["vbr", "bitrate"],
        help="MP3 encoding mode (default: %(default)s)",
    )
    parser.add_argument(
        "--mp3-vbr",
        type=int,
        default=0,
        help="MP3 VBR quality level, 0 is highest quality (default: %(default)s)",
    )
    parser.add_argument(
        "--timeout",
        default="30m",
        help="Per-item ytdl-pro timeout, e.g. 10m, 30m, 1h, or 0 to disable (default: %(default)s)",
    )
    parser.add_argument(
        "--retries",
        type=int,
        default=2,
        help="Retry count for transient failures per item (default: %(default)s)",
    )
    parser.add_argument(
        "--artist",
        default="",
        help="Optional artist tag override; if empty, a title heuristic is used when possible",
    )
    parser.add_argument(
        "--album",
        default="",
        help="Optional album tag override; if empty, the source playlist title is used",
    )
    parser.add_argument(
        "--date",
        default="",
        help="Optional date tag override, e.g. 2026",
    )
    parser.add_argument(
        "--comment",
        default="",
        help="Optional comment tag override",
    )
    parser.add_argument(
        "--overwrite",
        action="store_true",
        help="Pass -overwrite to ytdl-pro instead of preserving existing output files",
    )
    parser.add_argument(
        "--no-tag-metadata",
        action="store_true",
        help="Disable ffmpeg tagging after download",
    )
    return parser.parse_args()


def parse_duration(value: str) -> float | None:
    value = value.strip().lower()
    if value == "0":
        return None
    if value.endswith("ms"):
        return max(float(value[:-2]) / 1000.0, 0.0)

    total = 0.0
    number = ""
    for char in value:
        if char.isdigit() or char == ".":
            number += char
            continue
        if not number:
            raise ValueError(f"invalid duration {value!r}")
        amount = float(number)
        if char == "h":
            total += amount * 3600.0
        elif char == "m":
            total += amount * 60.0
        elif char == "s":
            total += amount
        else:
            raise ValueError(f"invalid duration unit {char!r} in {value!r}")
        number = ""

    if number:
        total += float(number)
    return total


def run_command(args: list[str], timeout: float | None = None) -> subprocess.CompletedProcess[str]:
    return subprocess.run(args, text=True, capture_output=True, timeout=timeout)


def fetch_playlist_batch(bin_path: str, playlist_url: str) -> tuple[str, list[PlaylistItem]]:
    proc = run_command([bin_path, "-url", playlist_url, "-list"])
    if proc.returncode != 0:
        raise RuntimeError(
            "failed to list playlist items:\n"
            + (proc.stderr.strip() or proc.stdout.strip() or "unknown error")
        )

    playlist_title = ""
    items: list[PlaylistItem] = []
    for line in proc.stdout.splitlines():
        if not playlist_title:
            playlist_match = PLAYLIST_RE.match(line)
            if playlist_match:
                playlist_title = playlist_match.group(1).strip()
                continue

        match = ITEM_RE.match(line)
        if match:
            items.append(
                PlaylistItem(
                    video_id=match.group(1),
                    title=match.group(2),
                    album=playlist_title,
                )
            )

    if not items:
        raise RuntimeError("no playlist items were parsed from ytdl-pro -list output")

    return playlist_title or "playlist", items


def load_archive(path: Path) -> set[str]:
    if not path.exists():
        return set()
    return {line.strip() for line in path.read_text(encoding="utf-8").splitlines() if line.strip()}


def append_archive(path: Path, video_id: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with path.open("a", encoding="utf-8") as handle:
        handle.write(video_id + "\n")


def sanitize_filename(value: str) -> str:
    value = SAFE_CHARS_RE.sub("_", value).strip()
    if not value:
        return "youtube-download"
    return value[:160]


def extension_for_audio_format(audio_format: str) -> str:
    mapping = {
        "mp3": ".mp3",
        "smart": ".mp3",
        "flac": ".flac",
        "wav": ".wav",
        "alac": ".m4a",
        "original": ".m4a",
    }
    return mapping.get(audio_format, ".mp3")


def split_stem_number(name: str) -> tuple[str, int | None]:
    match = re.match(r"^(.*) \((\d+)\)$", name)
    if not match:
        return name, None
    return match.group(1), int(match.group(2))


def find_existing_outputs(out_dir: Path, title: str, extension: str) -> list[Path]:
    base = sanitize_filename(title)
    matches: list[Path] = []
    if not out_dir.exists():
        return matches

    for path in out_dir.iterdir():
        if not path.is_file() or path.suffix.lower() != extension.lower():
            continue
        stem, _ = split_stem_number(path.stem)
        if stem == base:
            matches.append(path)
    return matches


def is_transient_error(message: str) -> bool:
    lower = message.lower()
    return any(pattern in lower for pattern in TRANSIENT_PATTERNS)


def infer_artist_and_title(raw_title: str) -> tuple[str, str]:
    for separator in (" - ", " – ", " — "):
        if separator not in raw_title:
            continue
        artist, song_title = raw_title.split(separator, 1)
        artist = artist.strip()
        song_title = song_title.strip()
        if artist and song_title and len(artist) <= 80:
            return artist, song_title
    return "", raw_title.strip()


def parse_downloaded_path(output: str) -> Path | None:
    candidate: Path | None = None
    for line in output.splitlines():
        match = DOWNLOADED_RE.match(line.strip())
        if match:
            candidate = Path(match.group(1).strip())
    return candidate


def output_format_for_path(path: Path) -> str | None:
    suffix = path.suffix.lower()
    if suffix == ".mp3":
        return "mp3"
    if suffix == ".m4a":
        return "ipod"
    if suffix == ".flac":
        return "flac"
    if suffix == ".wav":
        return "wav"
    return None


def metadata_fields(
    item: PlaylistItem,
    album_override: str,
    artist_override: str,
    date_override: str,
    comment_override: str,
) -> dict[str, str]:
    artist_guess, title_guess = infer_artist_and_title(item.title)
    fields = {
        "title": title_guess,
        "artist": artist_override.strip() or artist_guess,
        "album": album_override.strip() or item.album,
        "comment": comment_override.strip(),
        "date": date_override.strip(),
    }
    return {key: value for key, value in fields.items() if value}


def tag_audio_file(ffmpeg_path: str, input_path: Path, fields: dict[str, str]) -> tuple[bool, str]:
    if not input_path.exists():
        return False, f"tag source missing: {input_path}"

    output_format = output_format_for_path(input_path)
    temp_path = input_path.with_suffix(input_path.suffix + ".tagged")
    cmd = [ffmpeg_path, "-hide_banner", "-y", "-i", str(input_path), "-map", "0:a:0", "-c", "copy"]
    for key, value in fields.items():
        cmd.extend(["-metadata", f"{key}={value}"])
    if output_format:
        cmd.extend(["-f", output_format])
    cmd.append(str(temp_path))

    proc = subprocess.run(cmd, text=True, capture_output=True)
    if proc.returncode != 0:
        temp_path.unlink(missing_ok=True)
        message = (proc.stderr or proc.stdout or "").strip() or "ffmpeg metadata tagging failed"
        return False, message

    temp_path.replace(input_path)
    return True, ""


def download_one(
    bin_path: str,
    out_dir: Path,
    item: PlaylistItem,
    quality: str,
    audio_format: str,
    mp3_mode: str,
    mp3_vbr: int,
    timeout_arg: str,
    subprocess_timeout: float | None,
    overwrite: bool,
    max_attempts: int,
) -> Result:
    cmd = [
        bin_path,
        "-url",
        f"https://www.youtube.com/watch?v={item.video_id}",
        "-audio-only",
        "-audio-quality",
        quality,
        "-audio-format",
        audio_format,
        "-mp3-mode",
        mp3_mode,
        "-mp3-vbr",
        str(mp3_vbr),
        "-out",
        str(out_dir),
    ]
    if timeout_arg:
        cmd.extend(["-timeout", timeout_arg])
    if overwrite:
        cmd.append("-overwrite")

    attempts = 0
    last_message = ""
    while attempts <= max_attempts:
        attempts += 1
        try:
            proc = subprocess.run(cmd, text=True, capture_output=True, timeout=subprocess_timeout)
        except subprocess.TimeoutExpired:
            last_message = f"timeout after {subprocess_timeout} seconds"
            if attempts <= max_attempts:
                continue
            return Result(item.video_id, item.title, "failed", last_message, attempts)

        combined_output = f"{proc.stdout}\n{proc.stderr}".strip()
        if proc.returncode == 0:
            downloaded_path = parse_downloaded_path(combined_output)
            if downloaded_path is None:
                candidates = find_existing_outputs(out_dir, item.title, ".mp3")
                downloaded_path = candidates[-1] if candidates else None
            return Result(item.video_id, item.title, "downloaded", combined_output, attempts, downloaded_path)

        last_message = (proc.stderr or proc.stdout or "").strip()
        if not last_message:
            last_message = f"ytdl-pro exited with code {proc.returncode}"

        if attempts <= max_attempts and is_transient_error(last_message):
            continue

        return Result(item.video_id, item.title, "failed", last_message, attempts)

    return Result(item.video_id, item.title, "failed", last_message, attempts)


def print_progress(label: str, counters: Counters, active: int | None = None) -> None:
    active_count = counters.active if active is None else active
    print(
        f"[{label}] total={counters.total} completed={counters.completed} "
        f"downloaded={counters.downloaded} tagged={counters.tagged} "
        f"tag_failed={counters.tag_failed} skipped={counters.skipped} "
        f"failed={counters.failed} retried={counters.retried} active={active_count}",
        flush=True,
    )


def progress_reporter(
    stop_event: threading.Event,
    interval: int,
    label: str,
    counters: Counters,
    lock: threading.Lock,
) -> None:
    while not stop_event.wait(interval):
        with lock:
            print_progress(label, counters)


def main() -> int:
    args = parse_args()
    bin_path = Path(args.bin)
    if not bin_path.exists():
        print(f"error: ytdl-pro binary not found: {bin_path}", file=sys.stderr)
        return 2

    ffmpeg_path = shutil.which("ffmpeg")
    if not args.no_tag_metadata and not ffmpeg_path:
        print("error: ffmpeg not found on PATH; metadata tagging requires ffmpeg", file=sys.stderr)
        return 2

    timeout_arg = args.timeout.strip()
    subprocess_timeout = parse_duration(args.timeout)
    out_dir = Path(args.out).expanduser().resolve()
    out_dir.mkdir(parents=True, exist_ok=True)

    archive_path = Path(args.archive).expanduser().resolve() if args.archive else out_dir / ".downloaded_ids.txt"
    archive_ids = load_archive(archive_path)

    items_by_id: dict[str, PlaylistItem] = {}
    for playlist_url in args.playlists:
        playlist_title, items = fetch_playlist_batch(str(bin_path), playlist_url)
        for item in items:
            if item.video_id not in items_by_id:
                items_by_id[item.video_id] = dataclasses.replace(item, album=playlist_title)

    items = list(items_by_id.values())
    pending: list[PlaylistItem] = []
    skipped_archive = 0
    skipped_existing = 0
    expected_extension = extension_for_audio_format(args.audio_format)
    for item in items:
        if item.video_id in archive_ids:
            skipped_archive += 1
            continue
        if not args.overwrite and find_existing_outputs(out_dir, item.title, expected_extension):
            skipped_existing += 1
            continue
        pending.append(item)

    counters = Counters(
        total=len(items),
        skipped=skipped_archive + skipped_existing,
    )
    label = out_dir.name or "downloads"
    lock = threading.Lock()
    stop_event = threading.Event()

    print_progress(label, counters, active=0)
    if not pending:
        print(f"[{label}] nothing to download; every playlist item is already present or archived")
        return 0

    reporter = threading.Thread(
        target=progress_reporter,
        args=(stop_event, args.progress_interval, label, counters, lock),
        daemon=True,
    )
    reporter.start()

    failed_results: list[Result] = []
    try:
        with concurrent.futures.ThreadPoolExecutor(max_workers=args.workers) as executor:
            future_map = {
                executor.submit(
                    download_one,
                    str(bin_path),
                    out_dir,
                    item,
                    args.quality,
                    args.audio_format,
                    args.mp3_mode,
                    args.mp3_vbr,
                    timeout_arg,
                    subprocess_timeout,
                    args.overwrite,
                    args.retries,
                ): item
                for item in pending
            }

            for future in concurrent.futures.as_completed(future_map):
                item = future_map[future]
                try:
                    result = future.result()
                except Exception as exc:  # pragma: no cover - defensive
                    result = Result(item.video_id, item.title, "failed", str(exc), 1)

                tag_status = ""
                with lock:
                    counters.completed += 1
                    counters.active = max(len(pending) - counters.completed, 0)

                if result.status == "downloaded":
                    if result.path and not args.no_tag_metadata and ffmpeg_path:
                        fields = metadata_fields(item, args.album, args.artist, args.date, args.comment)
                        ok, tag_message = tag_audio_file(ffmpeg_path, result.path, fields)
                        with lock:
                            if ok:
                                counters.tagged += 1
                            else:
                                counters.tag_failed += 1
                                tag_status = f" tag_failed={tag_message.splitlines()[0]}"
                    with lock:
                        counters.downloaded += 1
                        append_archive(archive_path, result.video_id)
                else:
                    with lock:
                        counters.failed += 1
                        failed_results.append(result)
                        if result.attempts > 1:
                            counters.retried += result.attempts - 1

                print(
                    f"[{label}] {result.status}: {item.title} ({item.video_id}) [attempts={result.attempts}]{tag_status}",
                    flush=True,
                )
                if result.message:
                    preview = result.message.splitlines()[0]
                    print(f"[{label}] note: {preview}", flush=True)

                print_progress(label, counters, active=counters.active)
    finally:
        stop_event.set()

    print_progress(label, counters, active=0)

    if failed_results:
        print(f"[{label}] failed items:")
        for result in failed_results:
            preview = result.message.splitlines()[0] if result.message else "unknown error"
            print(f"  - {result.video_id}: {result.title} :: {preview}")

    return 0 if not failed_results else 1


if __name__ == "__main__":
    raise SystemExit(main())