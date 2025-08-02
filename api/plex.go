package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"emboxd/history"
	"emboxd/notification"

	"github.com/gin-gonic/gin"
)

// Plex webhook payload structure (simplified for movie events)
type plexNotification struct {
	Event string `json:"event"`
	User  bool   `json:"user"`
	Owner bool   `json:"owner"`
	Account struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Thumb string `json:"thumb,omitempty"`
	} `json:"Account"`
	Server struct {
		Title string `json:"title"`
		UUID  string `json:"uuid"`
	} `json:"Server"`
	Player struct {
		Local         bool   `json:"local"`
		PublicAddress string `json:"publicAddress"`
		Title         string `json:"title"`
		UUID          string `json:"uuid"`
	} `json:"Player"`
	Metadata struct {
		LibrarySectionType   string `json:"librarySectionType"`
		RatingKey            string `json:"ratingKey"`
		Key                  string `json:"key"`
		ParentRatingKey      string `json:"parentRatingKey,omitempty"`
		GrandparentRatingKey string `json:"grandparentRatingKey,omitempty"`
		Guid                 []struct {
			ID string `json:"id"`
		} `json:"Guid"`
		LibrarySectionID     int    `json:"librarySectionID"`
		Type                 string `json:"type"`
		Title                string `json:"title"`
		GrandparentKey       string `json:"grandparentKey,omitempty"`
		ParentKey            string `json:"parentKey,omitempty"`
		GrandparentTitle     string `json:"grandparentTitle,omitempty"`
		ParentTitle          string `json:"parentTitle,omitempty"`
		Summary              string `json:"summary"`
		Index                int    `json:"index,omitempty"`
		ParentIndex          int    `json:"parentIndex,omitempty"`
		RatingCount          int    `json:"ratingCount,omitempty"`
		Thumb                string `json:"thumb,omitempty"`
		Art                  string `json:"art,omitempty"`
		ParentThumb          string `json:"parentThumb,omitempty"`
		GrandparentThumb     string `json:"grandparentThumb,omitempty"`
		GrandparentArt       string `json:"grandparentArt,omitempty"`
		AddedAt              int64  `json:"addedAt"`
		UpdatedAt            int64  `json:"updatedAt"`
		Duration             int64  `json:"duration,omitempty"`
		ViewOffset           int64  `json:"viewOffset,omitempty"`
	} `json:"Metadata"`
}

// parsePlexImdbId takes an array of Guid objects and returns the first valid IMDb ID found
func parsePlexImdbId(guids []struct{ ID string }) string {
	if guids == nil {
		return ""
	}

	// Check each GUID for an IMDb ID
	for _, g := range guids {
		guid := g.ID
		// Direct IMDb format: "imdb://tt1234567"
		const imdbPrefix = "imdb://"
		if len(guid) > len(imdbPrefix) && guid[:len(imdbPrefix)] == imdbPrefix {
			return guid[len(imdbPrefix):]
		}

		// TMDb format: "tmdb://12345"
		// In a real implementation, this would do an API lookup to convert TMDb ID to IMDb ID
		const tmdbPrefix = "tmdb://"
		if len(guid) > len(tmdbPrefix) && guid[:len(tmdbPrefix)] == tmdbPrefix {
			// For now, just return the TMDb ID as a fallback
			// In a full implementation, you would:
			// 1. Extract the TMDb ID: tmdbId := guid[len(tmdbPrefix):]
			// 2. Use TMDb API to look up the corresponding IMDb ID
			// 3. Return the IMDb ID
			return guid[len(tmdbPrefix):]
		}

		// Plex internal format: "plex://movie/5d776b9..."
		const plexPrefix = "plex://"
		if len(guid) > len(plexPrefix) && guid[:len(plexPrefix)] == plexPrefix {
			// For now, just log and continue to next GUID
			slog.Debug("Plex internal ID found in GUID array, checking next available ID",
				slog.String("plex_guid", guid))
			continue
		}
	}

	// No valid IMDb ID found in any of the GUIDs
	return ""
}

func (a *Api) postPlexWebhook(context *gin.Context) {
	startTime := time.Now()

	// Track the webhook for metrics
	a.metrics.TrackWebhook("plex")

	// Read raw body for debugging
	body, _ := context.GetRawData()
	slog.Debug("Received Plex webhook payload", slog.String("raw_payload", string(body)))

	// Reset body for parsing
	context.Request.Body = io.NopCloser(bytes.NewBuffer(body))

	// Try to get JSON from form data first (Plex sends multipart form)
	var plexNotif plexNotification
	var err error
	
	// Check if it's multipart form data
	if strings.Contains(context.GetHeader("Content-Type"), "multipart/form-data") {
		// Parse as form data
		payloadStr := context.PostForm("payload")
		if payloadStr == "" {
			err = fmt.Errorf("no payload field in form data")
		} else {
			err = json.Unmarshal([]byte(payloadStr), &plexNotif)
		}
	} else {
		// Parse as direct JSON
		err = context.BindJSON(&plexNotif)
	}
	
	if err != nil {
		slog.Error("Malformed Plex webhook notification payload", slog.String("error", err.Error()), slog.String("payload", string(body)))

		// Log error event
		event := &history.Event{
			ID:           history.GenerateID(),
			Timestamp:    time.Now(),
			Type:         history.EventTypeWebhook,
			Source:       history.SourcePlex,
			Status:       history.StatusError,
			ErrorMessage: err.Error(),
			ProcessingMs: int(time.Since(startTime).Milliseconds()),
		}
		a.logEvent(event)

		context.AbortWithError(400, err)
		return
	}

	// Only handle movies with IMDB id
	if plexNotif.Metadata.Type != "movie" {
		context.AbortWithStatus(200)
		return
	}
	imdbId := parsePlexImdbId(plexNotif.Metadata.Guid)
	if imdbId == "" {
		context.AbortWithStatus(200)
		return
	}

	// Prefer using stable Account.id for user matching
	accountID := plexNotif.Account.ID
	username := plexNotif.Account.Title

	// Try to match by account ID first, then fall back to username
	var processor *notification.Processor
	var ok bool

	if accountID != 0 {
		// Convert int ID to string for lookup
		accountIDStr := fmt.Sprintf("%d", accountID)
		processor, ok = a.notificationProcessorByPlexAccountID[accountIDStr]
	}

	// Fall back to username matching if needed
	if !ok {
		processor, ok = a.notificationProcessorByPlexUsername[username]
	}

	if !ok {
		slog.Debug("No Letterboxd account for Plex user, ignoring notification",
			slog.Group("plex", "user", username, "accountID", accountID))
		context.AbortWithStatus(200)
		return
	}
	// Use current time since Plex webhooks don't include eventTime
	eventTime := time.Now()
	metadata := notification.Metadata{
		Server:   notification.Plex,
		Username: username,
		ImdbId:   imdbId,
		Time:     eventTime,
	}

	var eventType history.EventType

	switch plexNotif.Event {
	case "media.scrobble":
		watched := notification.WatchedNotification{
			Metadata: metadata,
			Watched:  true,
			Runtime:  time.Duration(plexNotif.Metadata.Duration) * time.Millisecond,
		}
		processor.ProcessWatchedNotification(watched)
		eventType = history.EventTypeWatched
	case "media.play", "media.resume":
		playback := notification.PlaybackNotification{
			Metadata: metadata,
			Playing:  true,
			Position: time.Duration(plexNotif.Metadata.ViewOffset) * time.Millisecond,
			Runtime:  time.Duration(plexNotif.Metadata.Duration) * time.Millisecond,
		}
		processor.ProcessPlaybackNotification(playback)
		eventType = history.EventTypePlayback
	case "media.pause", "media.stop":
		playback := notification.PlaybackNotification{
			Metadata: metadata,
			Playing:  false,
			Position: time.Duration(plexNotif.Metadata.ViewOffset) * time.Millisecond,
			Runtime:  time.Duration(plexNotif.Metadata.Duration) * time.Millisecond,
		}
		processor.ProcessPlaybackNotification(playback)
		eventType = history.EventTypePlayback
	default:
		context.AbortWithStatus(400)
		return
	}

	// Log successful event
	event := &history.Event{
		ID:         history.GenerateID(),
		Timestamp:  time.Now(),
		Type:       eventType,
		Source:     history.SourcePlex,
		Username:   username,
		MediaID:    imdbId,
		MediaTitle: plexNotif.Metadata.Title,
		Status:     history.StatusSuccess,
		Details: map[string]interface{}{
			"event":  plexNotif.Event,
			"server": plexNotif.Server.Title,
		},
		ProcessingMs: int(time.Since(startTime).Milliseconds()),
	}
	a.logEvent(event)

	context.Status(200)
}

func (a *Api) setupPlexRoutes() {
	plexRouter := a.router.Group("/plex")
	plexRouter.POST("/webhook", a.postPlexWebhook)
}
