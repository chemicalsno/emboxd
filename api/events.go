package api

import (
	"net/http"
	"strconv"

	"emboxd/history"

	"github.com/gin-gonic/gin"
)

// EventsResponse is the response format for the events endpoint
type EventsResponse struct {
	Total  int         `json:"total"`
	Limit  int         `json:"limit"`
	Events interface{} `json:"events"`
}

// getEvents returns the recent event history
func (a *Api) getEvents(context *gin.Context) {
	// Parse limit parameter
	limitParam := context.DefaultQuery("limit", "25")
	limit, err := strconv.Atoi(limitParam)
	if err != nil || limit <= 0 {
		limit = 25 // Default limit
	}

	// Get events from store
	events := a.eventHistory.GetLatest(limit)

	response := EventsResponse{
		Total:  len(events),
		Limit:  limit,
		Events: events,
	}

	context.JSON(http.StatusOK, response)
}

// logEvent adds an event to the history store
func (a *Api) logEvent(event interface{}) {
	if a.eventHistory != nil && event != nil {
		switch e := event.(type) {
		case *history.Event:
			a.eventHistory.Add(e)
		default:
			// Unsupported event type, ignore
		}
	}
}

// setupEventsRoutes sets up the events API routes
func (a *Api) setupEventsRoutes() {
	a.router.GET("/events", a.getEvents)
}
