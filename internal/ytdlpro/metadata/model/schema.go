package model

var AllowedActions = []string{
	"write_full",
	"write_partial",
	"write_base_only",
	"skip",
	"needs_review",
}

var RequiredFields = []string{
	"title",
	"artist",
	"album",
	"album_artist",
	"date",
	"genre",
	"label",
	"track_number",
	"disc_number",
	"musicbrainz_recording_id",
	"musicbrainz_release_id",
}
