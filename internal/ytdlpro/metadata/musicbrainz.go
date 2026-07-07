package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type musicBrainzClient struct {
	httpClient  *http.Client
	userAgent   string
	mu          sync.Mutex
	lastRequest time.Time
}

type musicBrainzSearchResponse struct {
	Recordings []struct {
		ID               string `json:"id"`
		Title            string `json:"title"`
		Length           int    `json:"length"`
		FirstReleaseDate string `json:"first-release-date"`
		ArtistCredit     []struct {
			Name string `json:"name"`
		} `json:"artist-credit"`
		Releases []struct {
			ID           string `json:"id"`
			Title        string `json:"title"`
			Date         string `json:"date"`
			ArtistCredit []struct {
				Name string `json:"name"`
			} `json:"artist-credit"`
			Media []struct {
				Position int `json:"position"`
				Track    []struct {
					Number string `json:"number"`
					Title  string `json:"title"`
				} `json:"track"`
			} `json:"media"`
		} `json:"releases"`
	} `json:"recordings"`
}

func newMusicBrainzClient() *musicBrainzClient {
	return &musicBrainzClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
		userAgent:  "ytdl-pro/metadata (+https://example.invalid)",
	}
}

func (c *musicBrainzClient) Lookup(ctx context.Context, base BaseMetadata, limit int) ([]Candidate, error) {
	if strings.TrimSpace(base.SearchTitle) == "" {
		return nil, nil
	}
	if err := c.throttle(ctx); err != nil {
		return nil, err
	}

	query := buildMusicBrainzQuery(base)
	endpoint := fmt.Sprintf("https://musicbrainz.org/ws/2/recording/?query=%s&fmt=json&limit=%d", url.QueryEscape(query), limit)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("musicbrainz lookup returned %s", resp.Status)
	}

	var payload musicBrainzSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode musicbrainz response: %w", err)
	}

	candidates := make([]Candidate, 0, len(payload.Recordings))
	for _, recording := range payload.Recordings {
		recordingArtist := joinArtistCredit(recording.ArtistCredit)
		if len(recording.Releases) == 0 {
			candidates = append(candidates, Candidate{
				CandidateID:            "musicbrainz:recording:" + recording.ID,
				Source:                 "musicbrainz",
				SourceURL:              "https://musicbrainz.org/recording/" + recording.ID,
				SourceTrust:            0.95,
				Title:                  strings.TrimSpace(recording.Title),
				Artist:                 recordingArtist,
				AlbumArtist:            recordingArtist,
				Date:                   strings.TrimSpace(recording.FirstReleaseDate),
				Year:                   parseYear(recording.FirstReleaseDate),
				DurationSeconds:        recording.Length / 1000,
				MusicBrainzRecordingID: recording.ID,
			})
			continue
		}

		for _, release := range recording.Releases {
			candidate := Candidate{
				CandidateID:            fmt.Sprintf("musicbrainz:recording:%s:release:%s", recording.ID, release.ID),
				Source:                 "musicbrainz",
				SourceURL:              "https://musicbrainz.org/recording/" + recording.ID,
				SourceTrust:            0.95,
				Title:                  strings.TrimSpace(recording.Title),
				Artist:                 recordingArtist,
				Album:                  strings.TrimSpace(release.Title),
				AlbumArtist:            joinArtistCredit(release.ArtistCredit),
				Date:                   firstCandidateValue(strings.TrimSpace(release.Date), strings.TrimSpace(recording.FirstReleaseDate)),
				Year:                   parseYear(firstCandidateValue(strings.TrimSpace(release.Date), strings.TrimSpace(recording.FirstReleaseDate))),
				DurationSeconds:        recording.Length / 1000,
				MusicBrainzRecordingID: recording.ID,
				MusicBrainzReleaseID:   release.ID,
			}
			if candidate.AlbumArtist == "" {
				candidate.AlbumArtist = recordingArtist
			}
			if len(release.Media) > 0 {
				candidate.DiscNumber = release.Media[0].Position
				if len(release.Media[0].Track) > 0 {
					candidate.TrackNumber = parseTrackNumber(release.Media[0].Track[0].Number)
				}
			}
			candidates = append(candidates, candidate)
		}
	}
	return candidates, nil
}

func buildMusicBrainzQuery(base BaseMetadata) string {
	parts := []string{fmt.Sprintf("recording:%q", base.SearchTitle)}
	if strings.TrimSpace(base.SearchArtist) != "" {
		parts = append(parts, fmt.Sprintf("artist:%q", base.SearchArtist))
	}
	return strings.Join(parts, " AND ")
}

func joinArtistCredit(credit []struct {
	Name string `json:"name"`
}) string {
	parts := make([]string, 0, len(credit))
	for _, part := range credit {
		if strings.TrimSpace(part.Name) != "" {
			parts = append(parts, strings.TrimSpace(part.Name))
		}
	}
	return strings.Join(parts, ", ")
}

func parseYear(value string) int {
	if len(value) < 4 {
		return 0
	}
	year, err := strconv.Atoi(value[:4])
	if err != nil {
		return 0
	}
	return year
}

func parseTrackNumber(value string) int {
	value = strings.TrimSpace(strings.Split(value, "/")[0])
	number, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return number
}

func firstCandidateValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (c *musicBrainzClient) throttle(ctx context.Context) error {
	c.mu.Lock()
	wait := time.Until(c.lastRequest.Add(time.Second))
	c.mu.Unlock()

	if wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}

	c.mu.Lock()
	c.lastRequest = time.Now()
	c.mu.Unlock()
	return nil
}
