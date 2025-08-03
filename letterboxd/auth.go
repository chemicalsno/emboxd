package letterboxd

import (
	"fmt"
	"log/slog"
	"os"
	"runtime"
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

	// Use a very short timeout for all login checks to avoid long waits
	// Save current timeout
	currentTimeout := 90000.0 // Default to 90 seconds if we can't get it
	// Set a shorter timeout for these operations
	activePage.SetDefaultTimeout(5000) // 5 seconds timeout for login checks
	// Reset timeout when we're done
	defer activePage.SetDefaultTimeout(currentTimeout)

	// Check multiple indicators of being logged in
	slog.Info("Checking login status with multiple indicators", slog.String("username", u.username))
	
	// 1. Check for user avatar (most reliable)
	slog.Debug("Checking for user avatar", slog.String("username", u.username))
	avatarLocator := activePage.Locator("#avatar")
	avatarVisible, err := avatarLocator.IsVisible()
	if err == nil && avatarVisible {
		slog.Info("User avatar found, user is logged in", slog.String("username", u.username))
		return true
	}

	// 2. Check for user menu
	slog.Debug("Checking for user menu", slog.String("username", u.username))
	userMenuSelectors := []string{
		".user-menu",
		"#userpanel.has-menu",
		".navitems li.logged-in",
	}
	
	for _, selector := range userMenuSelectors {
		userMenuLocator := activePage.Locator(selector)
		userMenuVisible, err := userMenuLocator.IsVisible()
		if err == nil && userMenuVisible {
			slog.Info("User menu element found, user is logged in", 
				slog.String("username", u.username),
				slog.String("selector", selector))
			return true
		}
	}

	// 3. Check for logged-in links that only appear when authenticated
	slog.Debug("Checking for authenticated-only links", slog.String("username", u.username))
	authLinks := []string{
		"a[href='/activity/']",
		"a[href='/films/diary/']",
		"a[href='/films/watchlist/']",
	}
	
	for _, selector := range authLinks {
		linkLocator := activePage.Locator(selector)
		linkVisible, err := linkLocator.IsVisible()
		if err == nil && linkVisible {
			slog.Info("Authenticated link found, user is logged in", 
				slog.String("username", u.username),
				slog.String("selector", selector))
			return true
		}
	}

	// 4. Check body class as a fallback, but with error handling
	slog.Debug("Checking body class as fallback", slog.String("username", u.username))
	try := func() (bool, error) {
		classes, err := activePage.Locator("body").GetAttribute("class")
		if err != nil {
			return false, err
		}
		return slices.Contains(strings.Split(classes, " "), "logged-in"), nil
	}
	
	// Try with a timeout to avoid hanging
	ch := make(chan bool, 1)
	errCh := make(chan error, 1)
	
	go func() {
		result, err := try()
		if err != nil {
			errCh <- err
		} else {
			ch <- result
		}
	}()
	
	// Wait with timeout
	select {
	case result := <-ch:
		slog.Info("Body class check result", 
			slog.Bool("logged_in", result), 
			slog.String("username", u.username))
		return result
	case err := <-errCh:
		slog.Error("Failed to get body class attribute",
			slog.String("error", err.Error()),
			slog.String("username", u.username))
	case <-time.After(3 * time.Second):
		slog.Warn("Body class check timed out", slog.String("username", u.username))
	}
	
	// 5. Check URL as a last resort
	currentUrl := activePage.URL()
	if strings.Contains(currentUrl, "/sign-in") {
		slog.Info("On sign-in page, user is not logged in", slog.String("username", u.username))
		return false
	}
	
	// Take a screenshot for debugging
	screenshotPath := "/tmp/letterboxd-login-check.png"
	if _, err := activePage.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(screenshotPath),
	}); err == nil {
		slog.Info("Saved login check screenshot", slog.String("path", screenshotPath))
	}
	
	// If we can't determine for sure, assume not logged in to trigger login attempt
	slog.Info("Could not definitively determine login status, assuming not logged in", 
		slog.String("username", u.username))
	return false
}

func (u User) Login() error {
	// Log the login attempt with detailed environment information
	slog.Info("Starting Letterboxd login process", 
		slog.String("username", u.username),
		slog.String("browser_path", os.Getenv("PLAYWRIGHT_BROWSERS_PATH")),
		slog.String("container_id", os.Getenv("HOSTNAME")),
		slog.String("go_version", runtime.Version()))
	
	// First check if already logged in to avoid unnecessary login attempts
	if u.isLoggedIn() {
		slog.Info("Already logged in, skipping login process", slog.String("username", u.username))
		return nil
	}

	// Use more aggressive retry config for login attempts
	config := RetryConfig{
		MaxAttempts:      5,         // Try more times
		InitialDelay:     time.Second * 3,  // Longer initial delay
		MaxDelay:         time.Second * 45, // Longer max delay
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
	slog.Info("Starting direct login flow", slog.String("username", u.username))
	
	// Use a longer timeout for initial page navigation
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

	// Check if we're already on the homepage (already logged in)
	currentURL := page.URL()
	slog.Info("Current URL after navigation", 
		slog.String("url", currentURL),
		slog.String("username", u.username))
	
	if currentURL == "https://letterboxd.com" || currentURL == "https://letterboxd.com/" {
		slog.Info("URL suggests we may already be logged in, verifying")
		
		// Double check we're actually logged in
		if u.isLoggedIn(page) {
			slog.Info("Already logged in", slog.String("username", u.username))
			return nil
		}
		
		// If not actually logged in but on homepage, navigate to login page
		slog.Info("Not logged in but on homepage, navigating to login page")
		if _, err := page.Goto("https://letterboxd.com/sign-in/"); err != nil {
			return &LetterboxdError{
				Type:          ErrorTypeNetwork,
				OriginalError: err,
				Context:       map[string]interface{}{"username": u.username, "url": "https://letterboxd.com/sign-in/"},
				Retryable:     true,
			}
		}
	}

	// Wait for the sign in page to be ready
	slog.Info("Waiting for sign-in page to be ready", slog.String("username", u.username))
	
	// Try multiple selectors to confirm we're on the login page
	readySelectors := []string{
		"input#username", 
		"input[name='username']",
		"form.signin-form",
		"h1.title:has-text('Sign In')",
	}
	
	var pageReady bool
	for _, selector := range readySelectors {
		locator := page.Locator(selector)
		visible, err := locator.IsVisible()
		if err == nil && visible {
			pageReady = true
			slog.Info("Login page ready", 
				slog.String("selector", selector),
				slog.String("username", u.username))
			break
		}
	}
	
	if !pageReady {
		slog.Warn("Login page may not be fully loaded", slog.String("username", u.username))
		// Take a screenshot to see what we're looking at
		screenshotPath := "/tmp/letterboxd-login-page-not-ready.png"
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{
			Path: playwright.String(screenshotPath),
		}); err == nil {
			slog.Info("Saved login page screenshot", slog.String("path", screenshotPath))
		}
	}

	// Use the robust fillLoginForm method
	if err := u.fillLoginForm(page); err != nil {
		slog.Error("Failed to fill login form", 
			slog.String("error", err.Error()),
			slog.String("username", u.username))
		return err
	}

	// Wait for successful login with a more reliable check
	slog.Info("Checking if login was successful", slog.String("username", u.username))
	if !u.isLoggedIn(page) {
		// Take a screenshot to diagnose the issue
		screenshotPath := "/tmp/letterboxd-login-failed.png"
		if _, screenshotErr := page.Screenshot(playwright.PageScreenshotOptions{
			Path: playwright.String(screenshotPath),
		}); screenshotErr == nil {
			slog.Info("Saved failed login screenshot", slog.String("path", screenshotPath))
		}
		
		// Check for specific error messages
		errorLocator := page.Locator("div.form-error")
		if errorVisible, _ := errorLocator.IsVisible(); errorVisible {
			errorText, _ := errorLocator.TextContent()
			slog.Error("Login failed with error message", 
				slog.String("error", errorText),
				slog.String("username", u.username))
			return &LetterboxdError{
				Type:          ErrorTypeAuth,
				OriginalError: fmt.Errorf("login failed: %s", errorText),
				Context:       map[string]interface{}{"username": u.username, "error_message": errorText},
				Retryable:     false, // Auth errors are not retryable
			}
		}
		
		// No specific error message found, report generic timeout
		slog.Error("Login timed out or failed without specific error", slog.String("username", u.username))
		return &LetterboxdError{
			Type:          ErrorTypeTimeout,
			OriginalError: fmt.Errorf("login verification timed out or failed"),
			Context:       map[string]interface{}{"username": u.username, "url": page.URL()},
			Retryable:     true,
		}
	}

	slog.Info("Successfully logged in", slog.String("username", u.username))
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
	
	// Take a screenshot of the login form for debugging
	screenshotPath := "/tmp/letterboxd-login-form.png"
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(screenshotPath),
	}); err == nil {
		slog.Info("Saved login form screenshot", slog.String("path", screenshotPath))
	}

	// Log current URL and page title for debugging
	currentUrl := page.URL()
	title, _ := page.Title()
	slog.Info("Login form page information", 
		slog.String("url", currentUrl),
		slog.String("title", title),
		slog.String("username", u.username))

	// Set a shorter timeout for form interactions to avoid hanging
	originalTimeout := 30000.0 // Default to 30 seconds
	page.SetDefaultTimeout(10000) // 10 seconds for form interactions
	defer page.SetDefaultTimeout(originalTimeout)

	// Wait for username field to be visible with multiple selectors
	usernameSelectors := []string{
		"input#username",
		"input[name='username']",
		"input[type='text'][required]",
	}
	
	var usernameLocator playwright.Locator
	var usernameFound bool
	
	for _, selector := range usernameSelectors {
		tmpLocator := page.Locator(selector)
		visible, err := tmpLocator.IsVisible()
		if err == nil && visible {
			usernameLocator = tmpLocator
			usernameFound = true
			slog.Info("Found username field", slog.String("selector", selector))
			break
		}
	}
	
	if !usernameFound {
		slog.Error("Username field not found or not visible", slog.String("username", u.username))
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: fmt.Errorf("username field not found"),
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	
	// Fill username field
	slog.Info("Filling username field", slog.String("username", u.username))
	
	// Clear the field first to ensure clean input
	if err := usernameLocator.Click(); err != nil {
		slog.Warn("Failed to click username field", slog.String("error", err.Error()))
	}
	
	if err := usernameLocator.Fill(""); err != nil {
		slog.Warn("Failed to clear username field", slog.String("error", err.Error()))
	}
	
	if err := usernameLocator.Fill(u.username); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	
	// Verify username was entered correctly
	usernameValue, err := usernameLocator.InputValue()
	if err == nil {
		slog.Info("Username field value", slog.String("value", usernameValue))
		if usernameValue != u.username {
			slog.Warn("Username field value mismatch, retrying", 
				slog.String("expected", u.username),
				slog.String("actual", usernameValue))
			
			// Try again with a different approach
			if err := usernameLocator.Fill(""); err == nil {
				// Type character by character
				for _, c := range u.username {
					if err := usernameLocator.Type(string(c), playwright.LocatorTypeOptions{
						Delay: playwright.Float(100), // 100ms delay between keystrokes
					}); err != nil {
						break
					}
				}
			}
		}
	}
	
	// Wait for password field to be visible with multiple selectors
	passwordSelectors := []string{
		"input#password",
		"input[name='password']",
		"input[type='password']",
	}
	
	var passwordLocator playwright.Locator
	var passwordFound bool
	
	for _, selector := range passwordSelectors {
		tmpLocator := page.Locator(selector)
		visible, err := tmpLocator.IsVisible()
		if err == nil && visible {
			passwordLocator = tmpLocator
			passwordFound = true
			slog.Info("Found password field", slog.String("selector", selector))
			break
		}
	}
	
	if !passwordFound {
		slog.Error("Password field not found or not visible", slog.String("username", u.username))
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: fmt.Errorf("password field not found"),
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	
	// Fill password field
	slog.Info("Filling password field")
	
	// Clear the field first
	if err := passwordLocator.Click(); err != nil {
		slog.Warn("Failed to click password field", slog.String("error", err.Error()))
	}
	
	if err := passwordLocator.Fill(""); err != nil {
		slog.Warn("Failed to clear password field", slog.String("error", err.Error()))
	}
	
	if err := passwordLocator.Fill(u.password); err != nil {
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: err,
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}
	
	// Try to check remember me box, but don't fail if it's not available
	rememberSelectors := []string{
		"input[name='remember']",
		"input.js-remember",
		"input[type='checkbox']",
	}
	
	for _, selector := range rememberSelectors {
		rememberLocator := page.Locator(selector)
		visible, err := rememberLocator.IsVisible()
		if err == nil && visible {
			slog.Info("Found remember checkbox", slog.String("selector", selector))
			if err := rememberLocator.Check(); err != nil {
				// Non-critical error, continue with login
				slog.Warn("Failed to check 'remember me' checkbox", 
					slog.String("error", err.Error()),
					slog.String("username", u.username))
			}
			break
		}
	}
	
	// Find and click submit button - try multiple possible selectors
	slog.Info("Clicking submit button")
	submitSelectors := []string{
		"input[type=submit]",
		"button[type=submit]",
		"button.submit",
		"input.submit",
		"button:has-text('Sign in')",
		"button:has-text('Log in')",
	}
	
	var submitErr error
	var clicked bool
	
	for _, selector := range submitSelectors {
		submitLocator := page.Locator(selector)
		visible, err := submitLocator.IsVisible()
		if err == nil && visible {
			slog.Info("Found submit button", slog.String("selector", selector))
			
			// Take a screenshot before clicking
			preClickScreenshotPath := "/tmp/letterboxd-pre-click.png"
			if _, err := page.Screenshot(playwright.PageScreenshotOptions{
				Path: playwright.String(preClickScreenshotPath),
			}); err == nil {
				slog.Info("Saved pre-click screenshot", slog.String("path", preClickScreenshotPath))
			}
			
			// Try click with different options if standard click fails
			if err := submitLocator.Click(); err == nil {
				clicked = true
				slog.Info("Successfully clicked submit button", slog.String("selector", selector))
				break
			} else {
				slog.Warn("Standard click failed, trying force click", 
					slog.String("error", err.Error()),
					slog.String("selector", selector))
				
				// Try force click
				if err := submitLocator.Click(playwright.LocatorClickOptions{
					Force: playwright.Bool(true),
				}); err == nil {
					clicked = true
					slog.Info("Successfully force-clicked submit button", slog.String("selector", selector))
					break
				} else {
					submitErr = err
				}
			}
		}
	}

	if !clicked {
		slog.Error("Failed to find and click submit button",
			slog.String("error", func() string {
				if submitErr != nil && submitErr.Error() != "" {
					return submitErr.Error()
				}
				return "no submit button found"
			}()),
			slog.String("username", u.username))
		return &LetterboxdError{
			Type:          ErrorTypeUI,
			OriginalError: func() error {
				if submitErr != nil {
					return submitErr
				}
				return fmt.Errorf("no submit button found")
			}(),
			Context:       map[string]interface{}{"username": u.username},
			Retryable:     true,
		}
	}

	// Wait for page navigation or form submission to complete
	slog.Info("Waiting for form submission to complete")

	// Wait a bit for the page to start loading after form submission
	time.Sleep(3 * time.Second)

	// Take a screenshot after form submission
	postSubmitScreenshotPath := "/tmp/letterboxd-post-submit.png"
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: playwright.String(postSubmitScreenshotPath),
	}); err == nil {
		slog.Info("Saved post-submit screenshot", slog.String("path", postSubmitScreenshotPath))
	}

	// Log current URL and page title after submission
	postSubmitUrl := page.URL()
	postSubmitTitle, _ := page.Title()
	slog.Info("Post-submission page information",
		slog.String("url", postSubmitUrl),
		slog.String("title", postSubmitTitle),
		slog.String("username", u.username))

	// Check for login errors first
	errorSelectors := []string{
		"div.form-error",
		".error-message",
		".alert-error",
	}

	for _, selector := range errorSelectors {
		errorLocator := page.Locator(selector)
		errorVisible, _ := errorLocator.IsVisible()
		if errorVisible {
			errorText, _ := errorLocator.TextContent()
			slog.Error("Login form error",
				slog.String("error", errorText),
				slog.String("selector", selector),
				slog.String("username", u.username))
			return &LetterboxdError{
				Type:          ErrorTypeAuth,
				OriginalError: fmt.Errorf("login failed: %s", errorText),
				Context:       map[string]interface{}{"username": u.username, "error_message": errorText},
				Retryable:     false, // Auth errors are not retryable
			}
		}
	}

	// Try multiple ways to confirm successful login with shorter timeouts
	successIndicators := []struct {
		name     string
		selector string
	}{
		{"body class", "body.logged-in"},
		{"avatar", "#avatar"},
		{"user menu", ".user-menu"},
		{"account link", "a[href='/settings/']"},
		{"activity link", "a[href='/activity/']"},
		{"watchlist link", "a[href='/films/watchlist/']"},
	}

	// Check for success indicators with shorter timeout
	for _, indicator := range successIndicators {
		slog.Info("Checking login success indicator",
			slog.String("indicator", indicator.name),
			slog.String("selector", indicator.selector))

		locator := page.Locator(indicator.selector)

		// First try a quick visibility check
		visible, _ := locator.IsVisible()
		if visible {
			slog.Info("Login successful - indicator found",
				slog.String("indicator", indicator.name),
				slog.String("username", u.username))
			return nil
		}

		// If not immediately visible, wait with a short timeout
		if err := locator.WaitFor(playwright.LocatorWaitForOptions{
			Timeout: playwright.Float(5000), // 5 second timeout for each indicator
		}); err == nil {
			slog.Info("Login successful - indicator appeared",
				slog.String("indicator", indicator.name),
				slog.String("username", u.username))
			return nil
		}
	}

	// Check if we're on a page that indicates successful login
	if !strings.Contains(postSubmitUrl, "/sign-in") {
		// We're no longer on the sign-in page, which is a good sign
		slog.Info("No longer on sign-in page, checking page content for login status", 
			slog.String("url", postSubmitUrl))
		
		// Try to get page content
		pageContent, err := page.Content()
		if err == nil {
			// Check for indicators in the HTML content
			if strings.Contains(pageContent, "logged-in") || 
			   strings.Contains(pageContent, "sign out") || 
			   strings.Contains(pageContent, "Sign out") {
				slog.Info("Login successful - found logged-in indicators in page content", 
					slog.String("username", u.username))
				return nil
			}
		}
		
		// As a last resort, if we're redirected to the home page or a film page
		// and not seeing any error messages, consider it a success
		if strings.Contains(postSubmitUrl, "/film/") || 
		   postSubmitUrl == "https://letterboxd.com/" || 
		   postSubmitUrl == "https://letterboxd.com" {
			slog.Info("Login likely successful - redirected to non-login page", 
				slog.String("url", postSubmitUrl),
				slog.String("username", u.username))
			return nil
		}
	}

	// If we get here, none of the success indicators were found
	slog.Error("Failed to confirm successful login", slog.String("username", u.username))
	
	// Take a final screenshot of the result page for debugging
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
