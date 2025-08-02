package api

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParsePlexImdbId(t *testing.T) {
	tests := []struct {
		name     string
		guid     string
		expected string
	}{
		{
			name:     "Valid IMDb GUID",
			guid:     "imdb://tt0133093",
			expected: "tt0133093",
		},
		{
			name:     "Valid TMDb GUID",
			guid:     "tmdb://27205",
			expected: "", // Currently returns empty as TMDb conversion is not implemented
		},
		{
			name:     "Valid Plex GUID",
			guid:     "plex://movie/5d776b9da7dcad001f89e688",
			expected: "", // Currently returns empty as Plex conversion is not implemented
		},
		{
			name:     "Empty GUID",
			guid:     "",
			expected: "",
		},
		{
			name:     "Invalid GUID format",
			guid:     "invalid://12345",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parsePlexImdbId(tt.guid)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPlexNotificationParsing(t *testing.T) {
	// Test cases with fixture files
	fixtures := []string{
		"testdata/plex_play_imdb.json",
		"testdata/plex_pause_imdb.json",
		"testdata/plex_scrobble_imdb.json",
		"testdata/plex_play_tmdb.json",
		"testdata/plex_play_plex.json",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			// Load fixture
			data, err := os.ReadFile(fixture)
			assert.NoError(t, err)

			// Parse JSON
			var notification plexNotification
			err = json.Unmarshal(data, &notification)
			assert.NoError(t, err)

			// Basic validation
			assert.NotEmpty(t, notification.Event)
			assert.NotEmpty(t, notification.Account.Title)
			assert.NotEmpty(t, notification.Metadata.Title)
			assert.NotEmpty(t, notification.Metadata.Guid)
			assert.Greater(t, notification.Metadata.Duration, int64(0))
			assert.NotEmpty(t, notification.Server.Title)
			assert.Greater(t, notification.EventTime, int64(0))
		})
	}
}