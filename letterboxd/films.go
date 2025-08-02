package letterboxd

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

func (u User) SetFilmWatched(imdbId string, watched bool) error {
	config := DefaultRetryConfig()
	op := fmt.Sprintf("SetFilmWatched(imdbId=%s, watched=%t)", imdbId, watched)

	return WithRetry(op, func() error {
		var url = fmt.Sprintf("https://letterboxd.com/imdb/%s", imdbId)
		var page = u.newPage(url)
		defer page.Close()

		// Reauthenticate if necessary
		if !u.isLoggedIn(page) {
			slog.Warn("Not logged in, authenticating...")

			loginErr := u.Login()
			if loginErr != nil {
				return &LetterboxdError{
					Type:          ErrorTypeAuth,
					OriginalError: loginErr,
					Context:       map[string]interface{}{"imdbId": imdbId},
					Retryable:     false,
				}
			}

			if _, err := page.Reload(); err != nil {
				return &LetterboxdError{
					Type:          ErrorTypeNetwork,
					OriginalError: err,
					Context:       map[string]interface{}{"url": url, "imdbId": imdbId},
					Retryable:     true,
				}
			}
		}

		// Allow watched information to populate
		time.Sleep(3 * time.Second)

		// Verify we're on the correct page
		pageTitle, _ := page.Title()
		pageURL, _ := page.URL()
		slog.Info("Letterboxd page loaded", 
			slog.String("imdbId", imdbId),
			slog.String("pageTitle", pageTitle),
			slog.String("pageURL", pageURL))

		// Find the watched button
		slog.Debug("Looking for watched button", slog.String("imdbId", imdbId), slog.String("selector", "span.action-large.-watch .action.-watch"))
		slog.Info("Attempting to mark film as watched on Letterboxd", slog.String("imdbId", imdbId))
		var watchedLocator = page.Locator("span.action-large.-watch .action.-watch")
		var classes, watchedLocatorErr = watchedLocator.GetAttribute("class")
		if watchedLocatorErr != nil {
			slog.Error("Failed to find watched button", slog.String("imdbId", imdbId), slog.String("error", watchedLocatorErr.Error()))
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: watchedLocatorErr,
				Context:       map[string]interface{}{"imdbId": imdbId, "selector": "span.action-large.-watch .action.-watch"},
				Retryable:     true,
			}
		}
		slog.Debug("Found watched button", slog.String("imdbId", imdbId), slog.String("classes", classes))

		if slices.Contains(strings.Split(classes, " "), "-on") == watched {
			// Film already marked with desired watch state
			slog.Info(fmt.Sprintf("Film %s is already marked as watched = %t", imdbId, watched))
			return nil
		} 
			
		// Toggle film watched status
		slog.Debug("Attempting to click watched button", slog.String("imdbId", imdbId))
		if err := watchedLocator.Click(); err != nil {
			slog.Error("Failed to click watched button", slog.String("imdbId", imdbId), slog.String("error", err.Error()))
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"imdbId": imdbId, "action": "click watched button"},
				Retryable:     true,
			}
		}
		slog.Debug("Successfully clicked watched button", slog.String("imdbId", imdbId))
		time.Sleep(3 * time.Second)
		
		slog.Info("Completed SetFilmWatched operation", slog.String("imdbId", imdbId), slog.Bool("watched", watched))
		return nil
	}, config)
}

func (u User) LogFilmWatched(imdbId string, date ...time.Time) error {
	if len(date) == 0 {
		date = append(date, time.Now())
	}

	config := DefaultRetryConfig()
	op := fmt.Sprintf("LogFilmWatched(imdbId=%s, date=%s)", imdbId, date[0].Format(time.DateOnly))

	return WithRetry(op, func() error {
		var url = fmt.Sprintf("https://letterboxd.com/imdb/%s", imdbId)
		var page = u.newPage(url)
		defer page.Close()

		// Reauthenticate if necessary
		if !u.isLoggedIn(page) {
			slog.Warn("Not logged in, authenticating...")

			loginErr := u.Login()
			if loginErr != nil {
				return &LetterboxdError{
					Type:          ErrorTypeAuth,
					OriginalError: loginErr,
					Context:       map[string]interface{}{"imdbId": imdbId},
					Retryable:     false,
				}
			}
			
			if _, err := page.Reload(); err != nil {
				return &LetterboxdError{
					Type:          ErrorTypeNetwork,
					OriginalError: err,
					Context:       map[string]interface{}{"url": url, "imdbId": imdbId},
					Retryable:     true,
				}
			}
		}

		// Click the 'Add to diary' button
		if err := page.Locator("button.add-this-film").Click(); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"imdbId": imdbId, "selector": "button.add-this-film"},
				Retryable:     true,
			}
		}

		// Wait for form to load
		var saveLocator = page.Locator("div#diary-entry-form-modal button.button.-action.button-action")
		if err := saveLocator.WaitFor(); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeTimeout,
				OriginalError: err,
				Context: map[string]interface{}{
					"imdbId": imdbId,
					"selector": "div#diary-entry-form-modal button.button.-action.button-action",
					"action": "wait for form",
				},
				Retryable: true,
			}
		}

		// Fill form and save log entry
		var javascriptSetDate = fmt.Sprintf("document.querySelector('input#frm-viewing-date-string').value = '%s'", date[0].Format(time.DateOnly))
		if _, err := page.Evaluate(javascriptSetDate, nil); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"imdbId": imdbId, "action": "set date"},
				Retryable:     true,
			}
		}
		
		if err := saveLocator.Click(); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"imdbId": imdbId, "action": "save diary entry"},
				Retryable:     true,
			}
		}
		
		time.Sleep(3 * time.Second)
		return nil
	}, config)
}
