package letterboxd

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/playwright-community/playwright-go"
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
		pageURL := page.URL()
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
		if page == nil {
			slog.Error("Failed to create page for Letterboxd", slog.String("imdbId", imdbId), slog.String("url", url))
			return &LetterboxdError{
				Type:          ErrorTypeNetwork,
				OriginalError: fmt.Errorf("failed to create page"),
				Context:       map[string]interface{}{"url": url, "imdbId": imdbId},
				Retryable:     true,
			}
		}
		defer page.Close()

		// Verify we're on the correct page
		pageTitle, _ := page.Title()
		pageURL := page.URL()
		slog.Info("Letterboxd page loaded for logging", 
			slog.String("imdbId", imdbId),
			slog.String("pageTitle", pageTitle),
			slog.String("pageURL", pageURL))

		// Reauthenticate if necessary
		if !u.isLoggedIn(page) {
			slog.Warn("Not logged in, authenticating...")

			loginErr := u.Login()
			if loginErr != nil {
				slog.Error("Failed to login", slog.String("imdbId", imdbId), slog.String("error", loginErr.Error()))
				return &LetterboxdError{
					Type:          ErrorTypeAuth,
					OriginalError: loginErr,
					Context:       map[string]interface{}{"imdbId": imdbId},
					Retryable:     false,
				}
			}
			
			if _, err := page.Reload(); err != nil {
				slog.Error("Failed to reload page after login", slog.String("imdbId", imdbId), slog.String("error", err.Error()))
				return &LetterboxdError{
					Type:          ErrorTypeNetwork,
					OriginalError: err,
					Context:       map[string]interface{}{"url": url, "imdbId": imdbId},
					Retryable:     true,
				}
			}
		}

		// Allow page to fully load
		time.Sleep(3 * time.Second)

		// Click the 'Review or log...' button
		slog.Info("Attempting to log film on Letterboxd", slog.String("imdbId", imdbId))
		slog.Debug("Looking for 'Review or log...' button", slog.String("imdbId", imdbId))
		
		// Try multiple possible selectors for the log button
		var logButtonSelectors = []string{
			"a:text('Review or log...')",
			".js-log-link",
			".js-add-to-diary",
			"a[href*='log']",
		}
		
		var clicked = false
		for _, selector := range logButtonSelectors {
			var logButton = page.Locator(selector)
			if logButton != nil {
				visible, _ := logButton.IsVisible()
				if visible {
					slog.Debug("Found log button", slog.String("selector", selector))
					if err := logButton.Click(); err == nil {
						clicked = true
						slog.Debug("Successfully clicked log button", slog.String("selector", selector))
						break
					}
				}
			}
		}
		
		if !clicked {
			slog.Error("Failed to find and click log button", slog.String("imdbId", imdbId))
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: fmt.Errorf("failed to find log button"),
				Context:       map[string]interface{}{"imdbId": imdbId},
				Retryable:     true,
			}
		}

		// Wait for form to load
		slog.Debug("Waiting for diary form to load", slog.String("imdbId", imdbId))
		time.Sleep(2 * time.Second)
		
		// Find the save button
		var saveButtonSelectors = []string{
			"button:text('SAVE')",
			".js-save-diary-entry",
			"button.button.-action.button-action",
			"div#diary-entry-form-modal button.button.-action.button-action",
		}
		
		var saveLocator *playwright.Locator
		var saveFound = false
		
		for _, selector := range saveButtonSelectors {
			saveLocator = page.Locator(selector)
			if saveLocator != nil {
				visible, _ := saveLocator.IsVisible()
				if visible {
					slog.Debug("Found save button", slog.String("selector", selector))
					saveFound = true
					break
				}
			}
		}
		
		if !saveFound {
			slog.Error("Failed to find save button", slog.String("imdbId", imdbId))
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: fmt.Errorf("failed to find save button"),
				Context:       map[string]interface{}{"imdbId": imdbId},
				Retryable:     true,
			}
		}

		// Fill form and save log entry
		slog.Debug("Setting date in diary form", slog.String("imdbId", imdbId), slog.String("date", date[0].Format(time.DateOnly)))
		var javascriptSetDate = fmt.Sprintf("document.querySelector('input#frm-viewing-date-string').value = '%s'", date[0].Format(time.DateOnly))
		if _, err := page.Evaluate(javascriptSetDate, nil); err != nil {
			slog.Error("Failed to set date", slog.String("imdbId", imdbId), slog.String("error", err.Error()))
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"imdbId": imdbId, "action": "set date"},
				Retryable:     true,
			}
		}
		
		// Make sure the watched checkbox is checked
		var watchedCheckbox = page.Locator("input[name='watched']")
		if watchedCheckbox != nil {
			checked, _ := watchedCheckbox.IsChecked()
			if !checked {
				slog.Debug("Checking watched checkbox", slog.String("imdbId", imdbId))
				if err := watchedCheckbox.Check(); err != nil {
					slog.Error("Failed to check watched checkbox", slog.String("imdbId", imdbId), slog.String("error", err.Error()))
				}
			}
		}
		
		// Click the save button
		slog.Debug("Clicking save button", slog.String("imdbId", imdbId))
		if err := saveLocator.Click(); err != nil {
			slog.Error("Failed to click save button", slog.String("imdbId", imdbId), slog.String("error", err.Error()))
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"imdbId": imdbId, "action": "save diary entry"},
				Retryable:     true,
			}
		}
		
		slog.Info("Successfully logged film as watched", slog.String("imdbId", imdbId), slog.String("date", date[0].Format(time.DateOnly)))
		time.Sleep(3 * time.Second)
		return nil
	}, config)
}
