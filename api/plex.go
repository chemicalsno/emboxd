package api

import (
	"log/slog"
	"time"

	"emboxd/history"
	"emboxd/notification"

	"github.com/gin-gonic/gin"
)

// Plex webhook payload structure (simplified for movie events)
type plexNotification struct {
	Event   string `json:"event"`
	Account struct {
		Title string `json:"title"`
		ID    string `json:"id"`
	} `json:"Account"`
	Metadata struct {
		Type       string `json:"type"`
		Title      string `json:"title"`
		Guid       string `json:"guid"`
		Duration   int64  `json:"duration"`
		ViewOffset int64  `json:"viewOffset"`
	} `json:"Metadata"`
	Player struct {
		State string `json:"state"`
	} `json:"Player"`
	Server struct {
		Title string `json:"title"`
	} `json:"Server"`
	EventTime int64 `json:"eventTime"`
}

func parsePlexImdbId(guid string) string {
	// Handle different GUID formats

	// Direct IMDb format: "imdb://tt1234567"
	const imdbPrefix = "imdb://"
	if len(guid) > len(imdbPrefix) && guid[:len(imdbPrefix)] == imdbPrefix {
		return guid[len(imdbPrefix):]
	}

	// TMDb format: "tmdb://12345"
	// In a real implementation, this would do an API lookup to convert TMDb ID to IMDb ID
	const tmdbPrefix = "tmdb://"
	if len(guid) > len(tmdbPrefix) && guid[:len(tmdbPrefix)] == tmdbPrefix {
		// For a full implementation, you would:
		// 1. Extract the TMDb ID: tmdbId := guid[len(tmdbPrefix):]
		// 2. Use TMDb API to look up the corresponding IMDb ID
		// 3. Return the IMDb ID

		// TODO: Implement TMDb to IMDb lookup
		// This would require an API call to something like:
		// https://api.themoviedb.org/3/movie/{tmdb_id}/external_ids

		slog.Debug("TMDb ID found but conversion to IMDb ID not yet implemented",
			slog.String("tmdb_guid", guid))
		return ""
	}

	// Plex internal format: "plex://movie/5d776b9..."
	const plexPrefix = "plex://"
	if len(guid) > len(plexPrefix) && guid[:len(plexPrefix)] == plexPrefix {
		// For a full implementation, you would need to:
		// 1. Extract the Plex item ID
		// 2. Query the Plex API to get external IDs for this item
		// 3. Return the IMDb ID if available

		slog.Debug("Plex internal ID found but conversion to IMDb ID not yet implemented",
			slog.String("plex_guid", guid))
		return ""
	}

	return ""
}

func (a *Api) postPlexWebhook(context *gin.Context) {
	startTime := time.Now()

	// Track the webhook for metrics
	a.metrics.TrackWebhook("plex")

	var plexNotif plexNotification
	if err := context.BindJSON(&plexNotif); err != nil {
		slog.Error("Malformed Plex webhook notification payload")

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

	if accountID != "" {
		processor, ok = a.notificationProcessorByPlexAccountID[accountID]
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
	eventTime := time.Unix(plexNotif.EventTime, 0)
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
