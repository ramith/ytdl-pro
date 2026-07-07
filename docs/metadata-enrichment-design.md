# Metadata Enrichment Design for `ytdl-pro`

## Purpose

Add an AI-assisted metadata enrichment pipeline to the Go downloader so downloaded audio files can be tagged with better metadata than the raw YouTube title alone.

The design focuses on:

- extracting base metadata from YouTube
- searching the web for corroborating metadata
- using a small local LLM to rank candidates and choose the most probable tag set
- writing tags into the downloaded file only when confidence is high enough
- keeping the feature integrated into the existing download path

This design assumes the user wants aggressive enrichment with thresholded writes, no persistent cache, and local model execution through Ollama or a similar local endpoint.

## Goals

1. Improve title, artist, album, year, genre, comment, and label/publisher tags for downloaded audio.
2. Prefer correct metadata over complete metadata.
3. Keep the feature deterministic enough to be safe in batch downloads.
4. Avoid re-downloading existing files.
5. Keep the implementation incremental and easy to disable.

## Non-Goals

- No attempt to build a perfect music recognition system.
- No persistent metadata database in v1.
- No automatic editing of audio waveforms or cover-art scraping in the first version.
- No requirement to call a cloud LLM if a local model is available.

## Chosen Architecture

The feature is implemented in the Go downloader path and works as a pipeline:

1. Download a file using `ytdl-pro`.
2. Collect base metadata from YouTube.
3. Build candidate metadata from search sources.
4. Normalize and score candidates.
5. Ask a small local LLM to rank candidates and produce a recommended tag set.
6. Apply a confidence threshold.
7. Tag the file with ffmpeg metadata.
8. Emit regular progress and an explainable decision summary.

The downloader remains the owner of file creation. Metadata enrichment is a service layered into the end of the download flow.

## Why This Shape

This approach balances accuracy and operational simplicity:

- YouTube metadata is always available, so it becomes the fallback.
- Web sources improve accuracy for common tracks, albums, and live releases.
- The local LLM only performs ranking and reconciliation, not free-form generation.
- Confidence gating prevents low-quality guesses from polluting output tags.

## Source Hierarchy

The system should collect metadata candidates from sources in this approximate trust order:

1. Official artist or label pages
2. MusicBrainz
3. Discogs
4. YouTube description or channel hints
5. Wikipedia as a weak fallback only
6. General web search results as discovery input, not as final truth

Google-style search is used to discover candidate pages and compare likely matches. Search results themselves are not treated as authoritative.

## Local LLM Choice

The model layer should be pluggable. For v1, the preferred backend is a local model served by Ollama.

Recommended local model candidates:

- Qwen2.5 3B Instruct
- Llama 3.2 3B Instruct
- Phi-3.5 Mini Instruct
- Gemma 2 2B Instruct
- Mistral 7B Instruct if more RAM and latency are acceptable

### Recommendation

Use a small model first, because this task is mostly classification, ranking, and normalization rather than long-form generation.

Suggested default:

- `Qwen2.5 3B Instruct` or `Llama 3.2 3B Instruct`

If accuracy is insufficient on noisy titles, allow a larger local model as a configuration swap.

## LLM Role

The model should not invent metadata from scratch. Its job is to:

- compare candidate metadata objects
- select the best matching candidate
- assign confidence
- explain the decision in one short paragraph or compact JSON fields

The model should receive only structured inputs, not raw web pages, after the source layer has already extracted facts.

## Metadata Fields

The first release should support writing the following tags where the container format allows it:

- `title`
- `artist`
- `album`
- `album artist`
- `date` or `year`
- `genre`
- `comment`
- `label` or `publisher` when a compatible tag field exists

### Practical Mapping

Not every audio container supports every field equally. The implementation should map fields by format:

- MP3: ID3 tags via ffmpeg / libmp3lame output tagging
- M4A: iTunes-style metadata atoms via ffmpeg
- FLAC: Vorbis comments
- WAV: minimal metadata support, best effort only

## Confidence Policy

Metadata is written only when the selected result exceeds a confidence threshold.

Suggested confidence bands:

- `0.90 - 1.00`: safe to write automatically
- `0.75 - 0.89`: write only if the sources agree strongly and the user enabled thresholded write
- `< 0.75`: do not write enriched metadata, fall back to YouTube metadata

The system should retain an explanation for why a track was accepted or rejected.

## Suggested Heuristic Scoring

Before the model is consulted, candidates should be given a deterministic pre-score.

Inputs to the heuristic score:

- title similarity to YouTube title
- artist match to channel name or description hints
- album match to playlist title or release page
- presence of multiple corroborating sources
- trust weight of each source
- release year consistency
- label/distributor consistency

Example weight ordering:

- official artist/label page: 1.0
- MusicBrainz: 0.95
- Discogs: 0.9
- YouTube channel/description: 0.7
- Wikipedia: 0.55
- general web search snippet: 0.3

The LLM then chooses among candidates after the heuristic stage narrows the set.

## Data Flow

### 1. Base Metadata Extraction

From YouTube, collect:

- title
- author/channel
- description
- playlist title if available
- video ID
- URL

### 2. Candidate Discovery

Use search queries derived from the YouTube title and channel:

- exact title search
- artist + title search
- title + album clue search
- title + label/publisher search

Search the web and collect structured candidate pages from the selected source hierarchy.

### 3. Source Parsing

Extract facts from source pages into a common candidate structure:

- track title
- artist
- album
- year
- label
- genre
- source URL
- source type
- trust weight

### 4. Candidate Ranking

Combine deterministic scoring and LLM ranking.

The LLM receives:

- YouTube metadata
- extracted source candidates
- heuristic scores
- a JSON schema for the output

The LLM returns:

- chosen candidate
- confidence
- short rationale
- optionally, per-field confidence

### 5. Tag Application

If confidence passes threshold:

- write tags into the output file using ffmpeg
- preserve the existing audio content
- replace the file atomically

If confidence does not pass:

- keep the YouTube-derived metadata only
- optionally log the rejected candidate set for review

## Integration Point in `ytdl-pro`

The feature should live inside the downloader path rather than as a separate post-processing script.

Recommended placement:

- after the audio file is downloaded
- before the final file is moved into place, or immediately after in an atomic rewrite step

This keeps the enrichment close to the file creation path and makes the decision visible in downloader logs.

## Proposed Go Package Structure

Suggested package split:

- `internal/ytdlpro/metadata/`
  - base metadata extraction
  - candidate normalization
  - confidence scoring
- `internal/ytdlpro/metadata/sources/`
  - MusicBrainz source adapter
  - Discogs source adapter
  - official page parser
  - general search adapter
- `internal/ytdlpro/metadata/model/`
  - local LLM client abstraction
  - JSON schema / prompt builder
- `internal/ytdlpro/tagging/`
  - ffmpeg metadata writer
  - format-specific tag mapping

This structure keeps web lookup, ranking, and file tagging separate.

## Prompt / Schema Design

The model should be given a compact JSON object.

Input schema:

```json
{
  "youtube": {
    "title": "...",
    "author": "...",
    "playlist_title": "...",
    "description": "..."
  },
  "candidates": [
    {
      "source": "MusicBrainz",
      "url": "https://...",
      "title": "...",
      "artist": "...",
      "album": "...",
      "year": "...",
      "label": "...",
      "trust": 0.95
    }
  ]
}
```

Output schema:

```json
{
  "selected_index": 0,
  "confidence": 0.93,
  "tags": {
    "title": "...",
    "artist": "...",
    "album": "...",
    "album_artist": "...",
    "year": "...",
    "genre": "...",
    "label": "..."
  },
  "rationale": "..."
}
```

The model should not be allowed to invent fields that are absent from the candidate pool unless the fallback is explicitly from YouTube metadata.

## Search Strategy

Use search only for discovery and validation.

Recommended query patterns:

- exact YouTube title in quotes
- artist + title
- title + official site
- title + MusicBrainz
- title + Discogs
- title + label/distributor

The search layer should capture:

- result title
- result URL
- snippet
- domain

Then the source adapters decide whether a result is usable.

## Avoiding Bad Matches

The system should reject candidates when:

- the artist differs materially from the YouTube channel hint
- the release year is implausible or inconsistent
- the track title is only a partial match with unrelated release context
- the source is low trust and no other source agrees
- the model confidence is below threshold

## Retry and Timeout Strategy

The downloader already has per-item timeout and retry support.

For metadata enrichment, apply a separate timeout budget:

- search timeout per query
- source parse timeout per page
- model inference timeout per item

If enrichment fails, the pipeline must continue with YouTube metadata.

## Caching

For the chosen configuration, no persistent cache is required in v1.

However, the design should still isolate cache usage behind an interface so it can be added later without changing the pipeline.

Potential future cache keys:

- video ID
- normalized title
- selected source URLs
- final tag decision

## Observability

The downloader should print concise progress plus enrichment diagnostics.

Useful log fields:

- video ID
- source count
- candidate count
- selected source
- confidence
- tag write status
- retry count
- fallback reason when metadata is not written

## Failure Modes

### 1. Search returns nothing

Fallback to YouTube metadata.

### 2. Web sources disagree

If no candidate exceeds the confidence threshold, do not write enriched tags.

### 3. LLM unavailable

Use deterministic heuristics only, or skip enrichment entirely if heuristic confidence is too low.

### 4. ffmpeg tagging fails

Keep the downloaded audio file and report the tagging failure separately.

### 5. Transient network or TLS issues

Retry with backoff for a limited number of attempts, then fall back to base metadata.

## Recommended Defaults

For your current setup, the recommended initial defaults are:

- model backend: Ollama
- model: Qwen2.5 3B Instruct or Llama 3.2 3B Instruct
- source policy: official pages, MusicBrainz, Discogs, label pages, Wikipedia fallback
- write policy: thresholded write
- cache: none for v1
- integration: downloader path
- metadata fields: title, artist, album, album artist, year, genre, comment, label

## Implementation Phases

### Phase 1: Base Tagging

- add ffmpeg metadata write support
- write YouTube-derived tags after download
- ensure atomic replace

### Phase 2: Source Adapters

- add MusicBrainz lookup
- add Discogs lookup
- add official page parsing
- add generic search discovery

### Phase 3: LLM Ranking

- add local model client abstraction
- implement JSON input/output schema
- add confidence scoring and selection logic

### Phase 4: Heuristic Hardening

- improve title parsing
- add label/album heuristics
- add failure explanations
- add optional debug mode for rejected candidates

### Phase 5: Optional Enhancements

- album art download and embedding
- persistent cache
- user review mode for low-confidence matches
- tag normalization rules per format

## Acceptance Criteria

The feature is ready when:

- a downloaded MP3 receives tags for title/artist/album when confidence is high enough
- the system skips tagging on low confidence instead of guessing
- playlist downloads continue even when metadata enrichment fails
- progress output remains visible during batch runs
- existing files are not re-downloaded
- the design stays local-model-first and does not require a cloud dependency

## Open Questions

1. Do you want album art support in v1 or later?
2. Should low-confidence tracks be written with base YouTube metadata or left untouched?
3. Do you want the script to expose a `--review-only` mode that prints the chosen metadata without writing it?
4. Should the Go binary itself expose the metadata enricher, or should the Python bulk script own the orchestration and call into the Go binary as a tagging backend?

## Summary

This design makes `ytdl-pro` a metadata-aware downloader without turning it into a full music library manager. The key idea is to combine YouTube metadata, curated web sources, and a small local LLM to produce probable tags only when confidence is high enough. That keeps the pipeline useful on messy real-world playlists while avoiding bad writes.