package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// HealthStatus represents the overall health status of the application
type HealthStatus struct {
	Status            string                  `json:"status"`
	Uptime            string                  `json:"uptime"`
	StartTime         time.Time               `json:"start_time"`
	LetterboxdWorkers []LetterboxdWorkerState `json:"letterboxd_workers"`
}

// LetterboxdWorkerState represents the status of a Letterboxd worker
type LetterboxdWorkerState struct {
	Username    string    `json:"username"`
	Connected   bool      `json:"connected"`
	LastChecked time.Time `json:"last_checked"`
}

var startTime = time.Now()

func (a *Api) getHealth(context *gin.Context) {
	status := HealthStatus{
		Status:    "ok",
		Uptime:    time.Since(startTime).Round(time.Second).String(),
		StartTime: startTime,
	}

	// Check Letterboxd workers status
	status.LetterboxdWorkers = make([]LetterboxdWorkerState, 0, len(a.letterboxdWorkers))
	allConnected := true

	for username, worker := range a.letterboxdWorkers {
		workerStatus := worker.CheckStatus()

		workerState := LetterboxdWorkerState{
			Username:    username,
			Connected:   workerStatus.IsConnected,
			LastChecked: workerStatus.LastChecked,
		}

		status.LetterboxdWorkers = append(status.LetterboxdWorkers, workerState)

		if !workerStatus.IsConnected {
			allConnected = false
		}
	}

	// If any Letterboxd worker is not connected, set status to warn
	if !allConnected {
		status.Status = "warning"
	}

	context.JSON(http.StatusOK, status)
}

func (a *Api) setupHealthRoutes() {
	a.router.GET("/health", a.getHealth)
	// Also handle HEAD requests for Docker healthchecks
	a.router.HEAD("/health", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
}
