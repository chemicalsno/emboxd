package history

import (
	"time"

	"emboxd/notification"
)

// EventType represents the type of event that was processed
type EventType string

const (
	// EventTypePlayback represents a playback notification (play, pause, resume)
	EventTypePlayback EventType = "playback"
	// EventTypeWatched represents a film being marked as watched
	EventTypeWatched EventType = "watched"
	// EventTypeWebhook represents a raw webhook received
	EventTypeWebhook EventType = "webhook"
)

// Source represents the source of the event
type Source string

const (
	// SourceEmby represents an event from Emby
	SourceEmby Source = "emby"
	// SourcePlex represents an event from Plex
	SourcePlex Source = "plex"
)

// Status represents the status of the event processing
type Status string

const (
	// StatusSuccess represents a successful event processing
	StatusSuccess Status = "success"
	// StatusError represents a failed event processing
	StatusError Status = "error"
	// StatusReceived represents an event that was received but not yet processed
	StatusReceived Status = "received"
)

// Event represents a single event in the history
type Event struct {
	ID           string                 `json:"id"`
	Timestamp    time.Time              `json:"timestamp"`
	Type         EventType              `json:"type"`
	Source       Source                 `json:"source"`
	Username     string                 `json:"username"`
	MediaID      string                 `json:"media_id,omitempty"`
	MediaTitle   string                 `json:"media_title,omitempty"`
	Status       Status                 `json:"status"`
	ErrorMessage string                 `json:"error_message,omitempty"`
	Details      map[string]interface{} `json:"details,omitempty"`
	ProcessingMs int                    `json:"processing_ms,omitempty"`
}

// FromNotification creates an Event from a notification
func FromNotification(notif interface{}, source Source, status Status, processingTime time.Duration, err error) *Event {
	event := &Event{
		ID:        GenerateID(),
		Timestamp: time.Now(),
		Source:    source,
		Status:    status,
		Details:   make(map[string]interface{}),
	}

	// Set processing time in milliseconds
	if processingTime > 0 {
		event.ProcessingMs = int(processingTime.Milliseconds())
	}

	// Set error message if applicable
	if err != nil {
		event.ErrorMessage = err.Error()
	}

	// Extract information based on notification type
	switch n := notif.(type) {
	case notification.PlaybackNotification:
		event.Type = EventTypePlayback
		event.Username = n.Metadata.Username
		event.MediaID = n.Metadata.ImdbId
		event.Details["playing"] = n.Playing
		event.Details["position"] = n.Position.String()
		event.Details["runtime"] = n.Runtime.String()
	case notification.WatchedNotification:
		event.Type = EventTypeWatched
		event.Username = n.Metadata.Username
		event.MediaID = n.Metadata.ImdbId
		event.Details["watched"] = n.Watched
		event.Details["runtime"] = n.Runtime.String()
	default:
		event.Type = EventTypeWebhook
		// For raw webhooks, we don't have structured data
	}

	return event
}

// GenerateID generates a simple ID for the event
func GenerateID() string {
	return time.Now().Format("20060102-150405.000")
}
