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
		guid     []struct{ ID string `json:"id"` }
		expected string
	}{
		{
			name:     "Valid IMDb GUID",
			guid:     []struct{ ID string `json:"id"` }{{ID: "imdb://tt0133093"}},
			expected: "tt0133093",
		},
		{
			name:     "Valid TMDb GUID",
			guid:     []struct{ ID string `json:"id"` }{{ID: "tmdb://27205"}},
			expected: "27205", // Now returns TMDb ID as fallback
		},
		{
			name:     "Valid Plex GUID",
			guid:     []struct{ ID string `json:"id"` }{{ID: "plex://movie/5d776b9da7dcad001f89e688"}},
			expected: "", // Currently returns empty as Plex conversion is not implemented
		},
		{
			name:     "Empty GUID",
			guid:     []struct{ ID string `json:"id"` }{},
			expected: "",
		},
		{
			name:     "Invalid GUID format",
			guid:     []struct{ ID string `json:"id"` }{{ID: "invalid://12345"}},
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
			// EventTime field was removed from the struct
			// assert.Greater(t, notification.EventTime, int64(0))
		})
	}
}