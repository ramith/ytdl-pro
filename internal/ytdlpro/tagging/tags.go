package tagging

import "strings"

type Tags struct {
	Title                  string
	Artist                 string
	Album                  string
	AlbumArtist            string
	Date                   string
	Year                   string
	Genre                  string
	Comment                string
	Label                  string
	TrackNumber            string
	DiscNumber             string
	MusicBrainzRecordingID string
	MusicBrainzReleaseID   string
}

func (t Tags) Clone() Tags {
	return t
}

func (t Tags) Empty() bool {
	return len(t.Values()) == 0
}

func (t Tags) Values() map[string]string {
	values := map[string]string{}
	if value := strings.TrimSpace(t.Title); value != "" {
		values["title"] = value
	}
	if value := strings.TrimSpace(t.Artist); value != "" {
		values["artist"] = value
	}
	if value := strings.TrimSpace(t.Album); value != "" {
		values["album"] = value
	}
	if value := strings.TrimSpace(t.AlbumArtist); value != "" {
		values["album_artist"] = value
	}
	if value := strings.TrimSpace(t.Date); value != "" {
		values["date"] = value
	}
	if value := strings.TrimSpace(t.Genre); value != "" {
		values["genre"] = value
	}
	if value := strings.TrimSpace(t.Comment); value != "" {
		values["comment"] = value
	}
	if value := strings.TrimSpace(t.Label); value != "" {
		values["label"] = value
	}
	if value := strings.TrimSpace(t.TrackNumber); value != "" {
		values["track_number"] = value
	}
	if value := strings.TrimSpace(t.DiscNumber); value != "" {
		values["disc_number"] = value
	}
	if value := strings.TrimSpace(t.MusicBrainzRecordingID); value != "" {
		values["musicbrainz_recording_id"] = value
	}
	if value := strings.TrimSpace(t.MusicBrainzReleaseID); value != "" {
		values["musicbrainz_release_id"] = value
	}
	return values
}

func (t Tags) Get(field string) string {
	switch field {
	case "title":
		return t.Title
	case "artist":
		return t.Artist
	case "album":
		return t.Album
	case "album_artist":
		return t.AlbumArtist
	case "date":
		return t.Date
	case "year":
		return t.Year
	case "genre":
		return t.Genre
	case "comment":
		return t.Comment
	case "label":
		return t.Label
	case "track_number":
		return t.TrackNumber
	case "disc_number":
		return t.DiscNumber
	case "musicbrainz_recording_id":
		return t.MusicBrainzRecordingID
	case "musicbrainz_release_id":
		return t.MusicBrainzReleaseID
	default:
		return ""
	}
}

func (t *Tags) Set(field, value string) {
	value = strings.TrimSpace(value)
	switch field {
	case "title":
		t.Title = value
	case "artist":
		t.Artist = value
	case "album":
		t.Album = value
	case "album_artist":
		t.AlbumArtist = value
	case "date":
		t.Date = value
	case "year":
		t.Year = value
	case "genre":
		t.Genre = value
	case "comment":
		t.Comment = value
	case "label":
		t.Label = value
	case "track_number":
		t.TrackNumber = value
	case "disc_number":
		t.DiscNumber = value
	case "musicbrainz_recording_id":
		t.MusicBrainzRecordingID = value
	case "musicbrainz_release_id":
		t.MusicBrainzReleaseID = value
	}
}
