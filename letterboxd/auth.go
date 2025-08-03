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
		slog.Info("Creating new page for login check", slog.String("username", u.username))
		activePage = u.newPage("https://letterboxd.com")
		if activePage == nil {
			slog.Error("Failed to create page for login check - browser not available", 
				slog.String("username", u.username))
			return false // Browser not available
		}
		shouldClosePage = true
	} else {
		activePage = page[0]
	}
	
	if shouldClosePage {
		defer activePage.Close()
	}

	// First try to find user avatar which is a reliable indicator of being logged in
	// and doesn't require checking body classes
	slog.Info("Checking for user avatar", slog.String("username", u.username))
	avatarLocator := activePage.Locator("#avatar")
	avatarVisible, err := avatarLocator.IsVisible()
	if err == nil && avatarVisible {
		slog.Info("User avatar found, user is logged in", slog.String("username", u.username))
		return true
	}

	// Also check for the user menu which appears when logged in
	userMenuLocator := activePage.Locator(".user-menu")
	userMenuVisible, err := userMenuLocator.IsVisible()
	if err == nil && userMenuVisible {
		slog.Info("User menu found, user is logged in", slog.String("username", u.username))
		return true
	}

	// Fall back to checking body class with a shorter timeout
	slog.Info("Checking body class for logged-in status", slog.String("username", u.username))
	
	// Set a shorter timeout for this operation to avoid long waits
	activePage.SetDefaultTimeout(15000) // 15 seconds timeout
	// Reset timeout after we're done (using a fixed value since GetDefaultTimeout is not available)
	defer activePage.SetDefaultTimeout(90000) // Reset to 90 seconds

	var classes, classErr = activePage.Locator("body").GetAttribute("class")
	if classErr != nil {
		slog.Error("Failed to get body class attribute",
			slog.String("error", classErr.Error()),
			slog.String("username", u.username))
		
		// Check URL as a last resort
		currentUrl := activePage.URL()
		if strings.Contains(currentUrl, "/sign-in") {
			slog.Info("On sign-in page, user is not logged in", slog.String("username", u.username))
			return false
		}
		
		// If we can't determine for sure, assume not logged in to trigger login attempt
		return false
	}

	isLoggedIn := slices.Contains(strings.Split(classes, " "), "logged-in")
	slog.Info("Body class check result", 
		slog.Bool("logged_in", isLoggedIn), 
		slog.String("username", u.username))
		
	return isLoggedIn
}

func (u User) Login() error {
	// First check if already logged in to avoid unnecessary login attempts
	if u.isLoggedIn() {
		slog.Info("Already logged in, skipping login process", slog.String("username", u.username))
		return nil
	}

	// Use more aggressive retry config for login attempts
	config := RetryConfig{
		MaxAttempts:      5,         // Try more times
		InitialDelay:     time.Second * 2,
		MaxDelay:         time.Second * 30,
		BackoffFactor:    2.0,
		RetryableErrors:  []ErrorType{ErrorTypeNetwork, ErrorTypeTimeout, ErrorTypeUI},
	}
	op := fmt.Sprintf("Login(username=%s)", u.username)

	return WithRetry(op, func() error {
		// Try all login flows in sequence until one succeeds
		var loginErr error
		
		// First try direct login
		slog.Info("Attempting direct login", slog.String("username", u.username))
		loginErr = u.directLogin()
		if loginErr == nil {
			slog.Info("Direct login successful", slog.String("username", u.username))
			return nil
		}
		
		// If direct login fails, try alternative login flow via film page
		slog.Info("Direct login failed, trying alternative login flow 1", 
			slog.String("error", loginErr.Error()),
			slog.String("username", u.username))
		
		loginErr = u.alternativeLogin()
		if loginErr == nil {
			slog.Info("Alternative login flow 1 successful", slog.String("username", u.username))
			return nil
		}
		
		// If alternative login fails, try header menu login flow
		slog.Info("Alternative login flow 1 failed, trying alternative login flow 2", 
			slog.String("error", loginErr.Error()),
			slog.String("username", u.username))
		
		loginErr = u.headerMenuLogin()
		if loginErr == nil {
			slog.Info("Alternative login flow 2 successful", slog.String("username", u.username))
			return nil
		}
		
		slog.Error("All login flows failed", 
			slog.String("error", loginErr.Error()),
			slog.String("username", u.username))
		return loginErr
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
	
	// Check if we're still on the film page (modal login) or redirected to sign-in page
	currentUrl := page.URL()
	slog.Info("Current URL after clicking sign-in link", slog.String("url", currentUrl))
	
	// If we're redirected to the sign-in page, we need to handle that differently
	if strings.Contains(currentUrl, "/sign-in") {
		slog.Info("Redirected to sign-in page, handling regular login form")
		return u.fillLoginForm(page)
	}
	
	// We're still on the film page, look for the modal login form
	slog.Info("Looking for modal login form on film page")
	
	// Try to find the modal login form
	modalSelectors := []string{
		"#modal",
		".modal",
		".modal-content",
		"div[role='dialog']",
	}
	
	var modalFound = false
	for _, selector := range modalSelectors {
		modalLocator := page.Locator(selector)
		if modalLocator != nil {
			visible, _ := modalLocator.IsVisible()
			if visible {
				slog.Info("Found modal dialog", slog.String("selector", selector))
				modalFound = true
				break
			}
		}
	}
	
	// If modal not found, try to find any login form elements directly
	if !modalFound {
		slog.Info("Modal not found, looking for login form elements directly")
		
		// Check for username field
		usernameLocator := page.Locator("input#username")
		if usernameVisible, _ := usernameLocator.IsVisible(); usernameVisible {
			slog.Info("Found username field directly on page")
			modalFound = true
		}
	}
	
	// If we still can't find a login form, try to navigate directly to the sign-in page
	if !modalFound {
		slog.Warn("Could not find modal login form, navigating directly to sign-in page")
		if _, err := page.Goto("https://letterboxd.com/sign-in/", playwright.PageGotoOptions{
			WaitUntil: playwright.WaitUntilStateNetworkidle,
		}); err != nil {
			slog.Error("Failed to navigate to sign-in page", slog.String("error", err.Error()))
			return &LetterboxdError{
				Type:          ErrorTypeNetwork,
				OriginalError: err,
				Context:       map[string]interface{}{"username": u.username},
				Retryable:     true,
			}
		}
	}
	
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
	// Set a shorter timeout for form interactions to avoid long waits
	page.SetDefaultTimeout(30000) // 30 seconds timeout for form interactions
	// Reset timeout after we're done
	defer page.SetDefaultTimeout(90000) // Reset to 90 seconds

	slog.Info("Filling login form", slog.String("username", u.username))
	
	// Take screenshot of the login form for debugging
	screenshotPath := "/tmp/letterboxd-login-form.png"
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(screenshotPath),
	}); err == nil {
		slog.Info("Saved login form screenshot", slog.String("path", screenshotPath))
	}
	
	// Wait for username field to be visible
	usernameLocator := page.Locator("input#username")
	if err := usernameLocator.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		slog.Error("Username field not found or not visible", 
			slog.String("error", err.Error()),
			slog.String("username", u.username))
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#username"},
			Retryable:     true,
		}
	}
	
	// Fill username field
	slog.Info("Filling username field", slog.String("username", u.username))
	if err := usernameLocator.Fill(u.username); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#username"},
			Retryable:     true,
		}
	}
	
	// Wait for password field to be visible
	passwordLocator := page.Locator("input#password")
	if err := passwordLocator.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible,
	}); err != nil {
		slog.Error("Password field not found or not visible", 
			slog.String("error", err.Error()),
			slog.String("username", u.username))
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#password"},
			Retryable:     true,
		}
	}
	
	// Fill password field
	slog.Info("Filling password field")
	if err := passwordLocator.Fill(u.password); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username, "selector": "input#password"},
			Retryable:     true,
		}
	}
	
	// Try to check remember me box, but don't fail if it's not available
	rememberLocator := page.Locator("input[name='remember']")
	if visible, _ := rememberLocator.IsVisible(); visible {
		if err := rememberLocator.Check(); err != nil {
			// Non-critical error, continue with login
			slog.Warn("Failed to check 'remember me' checkbox", 
				slog.String("error", err.Error()),
				slog.String("username", u.username))
		}
	}
	
	// Find and click submit button - try multiple possible selectors
	slog.Info("Clicking submit button")
	submitSelectors := []string{
		"input[type=submit]",
		"button[type=submit]",
		"button.submit",
		"input.submit",
	}
	
	var submitErr error
	var clicked bool
	
	for _, selector := range submitSelectors {
		submitLocator := page.Locator(selector)
		visible, _ := submitLocator.IsVisible()
		if visible {
			slog.Info("Found submit button", slog.String("selector", selector))
			if err := submitLocator.Click(); err == nil {
				clicked = true
				break
			} else {
				submitErr = err
			}
		}
	}
	
	if !clicked {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: submitErr,
			Context:       map[string]interface{}{"username": u.username, "selector": "submit button"},
			Retryable:     true,
		}
	}

	// Wait for logged in status with multiple indicators
	slog.Info("Waiting for successful login confirmation")
	
	// Try multiple ways to confirm successful login
	successIndicators := []struct {
		name     string
		selector string
	}{
		{"body class", "body.logged-in"},
		{"avatar", "#avatar"},
		{"user menu", ".user-menu"},
	}
	
	// Wait a bit for the page to start loading after form submission
	time.Sleep(2 * time.Second)
	
	// Check for login errors first
	errorLocator := page.Locator("div.form-error")
	if errorVisible, _ := errorLocator.IsVisible(); errorVisible {
		errorText, _ := errorLocator.TextContent()
		slog.Error("Login form error", 
			slog.String("error", errorText),
			slog.String("username", u.username))
		return &LetterboxdError{
			Type:          ErrorTypeAuth,
			OriginalError: fmt.Errorf("login failed: %s", errorText),
			Context:       map[string]interface{}{"username": u.username, "error_message": errorText},
			Retryable:     false, // Auth errors are not retryable
		}
	}
	
	// Check for success indicators
	for _, indicator := range successIndicators {
		slog.Info("Checking login success indicator", 
			slog.String("indicator", indicator.name),
			slog.String("selector", indicator.selector))
		
		locator := page.Locator(indicator.selector)
		if err := locator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(15000), // 15 second timeout for each indicator
		}); err == nil {
			slog.Info("Login successful", 
				slog.String("indicator", indicator.name),
				slog.String("username", u.username))
			return nil
		}
	}

	// If we get here, none of the success indicators were found
	slog.Error("Failed to confirm successful login", slog.String("username", u.username))
	
	// Take screenshot of the result page for debugging
	resultScreenshotPath := "/tmp/letterboxd-login-result.png"
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(resultScreenshotPath),
	}); err == nil {
		slog.Info("Saved login result screenshot", slog.String("path", resultScreenshotPath))
	}
	
	return &LetterboxdError{
		Type:          ErrorTypeTimeout,
		OriginalError: fmt.Errorf("could not confirm successful login"),
		Context:       map[string]interface{}{"username": u.username},
		Retryable:     true,
	}
}
