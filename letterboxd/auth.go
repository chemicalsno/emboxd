package letterboxd

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

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
		// Try all login flows in sequence until one succeeds
		var loginErr error
		
		// First try direct login
		loginErr = u.directLogin()
		if loginErr == nil {
			return nil
		}
		
		// If direct login fails, try alternative login flow via film page
		slog.Info("Direct login failed, trying alternative login flow 1", 
			slog.String("error", loginErr.Error()),
			slog.String("username", u.username))
		
		loginErr = u.alternativeLogin()
		if loginErr == nil {
			return nil
		}
		
		// If alternative login fails, try header menu login flow
		slog.Info("Alternative login flow 1 failed, trying alternative login flow 2", 
			slog.String("error", loginErr.Error()),
			slog.String("username", u.username))
		
		return u.headerMenuLogin()
	}, config)
}

func (u User) directLogin() error {
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
	if err := page.Locator("input#username").Fill(u.username); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#username"},
			Retryable:     true,
		}
	}
	
	if err := page.Locator("input#password").Fill(u.password); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#password"},
			Retryable:     true,
		}
	}
	
	if err := page.Locator("input[name='remember']").Check(); err != nil {
		// Non-critical error, continue with login
		slog.Warn("Failed to check 'remember me' checkbox", 
			slog.String("error", err.Error()),
			slog.String("username", u.username))
	}
	
	if err := page.Locator("input[type=submit]").Click(); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input[type=submit]"},
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
}

func (u User) alternativeLogin() error {
	// Start from a film page and use the "Sign in to log, rate or review" link
	var page = u.newPage("https://letterboxd.com/film/bubba-ho-tep/")
	if page == nil {
		return &LetterboxdError{
			Type:          ErrorTypeNetwork,
			OriginalError: fmt.Errorf("failed to create page - browser not available"),
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	defer page.Close()

	// Check if already logged in
	if u.isLoggedIn(page) {
		slog.Info("Already logged in")
		return nil
	}
	
	// Click on "Sign in to log, rate or review" link
	slog.Info("Clicking on 'Sign in to log, rate or review' link")
	
	// Try multiple possible selectors for the sign-in link
	var signInSelectors = []string{
		"li.panel-signin > a",
		"a:text('Sign in to log')",
		"#userpanel a:first-child",
	}
	
	var clicked = false
	for _, selector := range signInSelectors {
		var signInLink = page.Locator(selector)
		if signInLink != nil {
			visible, _ := signInLink.IsVisible()
			if visible {
				slog.Debug("Found sign-in link", slog.String("selector", selector))
				if err := signInLink.Click(); err == nil {
					clicked = true
					slog.Debug("Successfully clicked sign-in link")
					break
				}
			}
		}
	}
	
	if !clicked {
		slog.Error("Failed to find and click sign-in link")
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: fmt.Errorf("failed to find sign-in link"),
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	
	// Wait for login form to appear
	time.Sleep(2 * time.Second)
	
	return u.fillLoginForm(page)
}

func (u User) headerMenuLogin() error {
	// Start from a film page and use the header "Sign In" menu
	var page = u.newPage("https://letterboxd.com/film/bubba-ho-tep/")
	if page == nil {
		return &LetterboxdError{
			Type:          ErrorTypeNetwork,
			OriginalError: fmt.Errorf("failed to create page - browser not available"),
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	defer page.Close()

	// Check if already logged in
	if u.isLoggedIn(page) {
		slog.Info("Already logged in")
		return nil
	}
	
	// Click on header "Sign In" menu
	slog.Info("Clicking on header 'Sign In' menu")
	
	// Try multiple possible selectors for the header sign-in link
	var headerSignInSelectors = []string{
		"li.sign-in-menu span.label",
		"#header nav li.sign-in-menu a",
		"a:has-text('Sign In')",
	}
	
	var clicked = false
	for _, selector := range headerSignInSelectors {
		var signInLink = page.Locator(selector)
		if signInLink != nil {
			visible, _ := signInLink.IsVisible()
			if visible {
				slog.Debug("Found header sign-in link", slog.String("selector", selector))
				if err := signInLink.Click(); err == nil {
					clicked = true
					slog.Debug("Successfully clicked header sign-in link")
					break
				}
			}
		}
	}
	
	if !clicked {
		slog.Error("Failed to find and click header sign-in link")
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: fmt.Errorf("failed to find header sign-in link"),
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	
	// Wait for login form to appear
	time.Sleep(2 * time.Second)
	
	return u.fillLoginForm(page)
}

// Common function to fill and submit the login form
func (u User) fillLoginForm(page playwright.Page) error {
	// Fill out login form
	if err := page.Locator("input#username").Fill(u.username); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#username"},
			Retryable:     true,
		}
	}
	
	if err := page.Locator("input#password").Fill(u.password); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#password"},
			Retryable:     true,
		}
	}
	
	if err := page.Locator("input[name='remember']").Check(); err != nil {
		// Non-critical error, continue with login
		slog.Warn("Failed to check 'remember me' checkbox", 
			slog.String("error", err.Error()),
			slog.String("username", u.username))
	}
	
	if err := page.Locator("input[type=submit]").Click(); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input[type=submit]"},
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
}
