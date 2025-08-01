package letterboxd

import (
	"log/slog"
	"time"
)

// Status represents the current status of a Letterboxd worker
type Status struct {
	Username    string `json:"username"`
	IsConnected bool   `json:"is_connected"`
	LastChecked time.Time `json:"last_checked"`
}

// CheckStatus tests the connection to Letterboxd and returns the current status
func (w *Worker) CheckStatus() Status {
	// Create a page to check login status
	var page = w.user.newPage("https://letterboxd.com")
	defer page.Close()

	// Check if the user is logged in
	isLoggedIn := w.user.isLoggedIn(page)
	
	status := Status{
		Username:    w.user.username,
		IsConnected: isLoggedIn,
		LastChecked: time.Now(),
	}

	if !isLoggedIn {
		slog.Warn("Letterboxd worker not logged in", slog.String("username", w.user.username))
		// Attempt to login again if not connected
		go func() {
			w.user.Login()
		}()
	}

	return status
}