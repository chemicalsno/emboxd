package letterboxd

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/playwright-community/playwright-go"
)

func (u User) isLoggedIn(page ...playwright.Page) bool {
	var shouldClosePage bool
	var activePage playwright.Page
	
	if len(page) == 0 {
		// Create new page
		activePage = u.newPage("https://letterboxd.com")
		if activePage == nil {
			return false // Browser not available
		}
		shouldClosePage = true
	} else {
		activePage = page[0]
	}
	
	if shouldClosePage {
		defer activePage.Close()
	}

	var classes, err = activePage.Locator("body").GetAttribute("class")
	if err != nil {
		slog.Error("Failed to get body class attribute",
			slog.String("error", err.Error()),
			slog.String("username", u.username))
		return false
	}

	return slices.Contains(strings.Split(classes, " "), "logged-in")
}

func (u User) Login() error {
	config := DefaultRetryConfig()
	op := fmt.Sprintf("Login(username=%s)", u.username)

	return WithRetry(op, func() error {
		var page = u.newPage("https://letterboxd.com/sign-in/")
		if page == nil {
			return &LetterboxdError{
				Type:          ErrorTypeNetwork,
				OriginalError: fmt.Errorf("failed to create page - browser not available"),
				Context:       map[string]interface{}{"username": u.username},
				Retryable:     true,
			}
		}
		defer page.Close()

		if page.URL() == "https://letterboxd.com" {
			slog.Info("Already logged in")
			return nil
		}

		// Fill out login form
		if err := page.Locator("input#field-username").Fill(u.username); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"username": u.username, "selector": "input#field-username"},
				Retryable:     true,
			}
		}
		
		if err := page.Locator("input#field-password").Fill(u.password); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"username": u.username, "selector": "input#field-password"},
				Retryable:     true,
			}
		}
		
		if err := page.Locator("input.js-remember").Check(); err != nil {
			// Non-critical error, continue with login
			slog.Warn("Failed to check 'remember me' checkbox", 
				slog.String("error", err.Error()),
				slog.String("username", u.username))
		}
		
		if err := page.Locator("div.formbody > div.formrow > button[type=submit]").Click(); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeUI,
				OriginalError: err,
				Context:       map[string]interface{}{"username": u.username, "selector": "button[type=submit]"},
				Retryable:     true,
			}
		}

		// Wait for logged in status
		if err := page.Locator("body.logged-in").WaitFor(); err != nil {
			// Check if there's a login error message
			errorLocator := page.Locator("div.form-error")
			if errorVisible, _ := errorLocator.IsVisible(); errorVisible {
				errorText, _ := errorLocator.TextContent()
				return &LetterboxdError{
					Type:          ErrorTypeAuth,
					OriginalError: fmt.Errorf("login failed: %s", errorText),
					Context:       map[string]interface{}{"username": u.username, "error_message": errorText},
					Retryable:     false, // Auth errors are not retryable
				}
			}
			
			return &LetterboxdError{
				Type:          ErrorTypeTimeout,
				OriginalError: err,
				Context:       map[string]interface{}{"username": u.username, "selector": "body.logged-in"},
				Retryable:     true,
			}
		}

		slog.Info(fmt.Sprintf("Logged in as %s", u.username))
		return nil
	}, config)
}
