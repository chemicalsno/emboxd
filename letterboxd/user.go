package letterboxd

import (
	"log/slog"
	"os"
	"os/exec"
	"time"

	"github.com/playwright-community/playwright-go"
)

// Global variables for Playwright and browser instance
var pw *playwright.Playwright
var browser playwright.Browser
var browserLaunchOptions playwright.BrowserTypeLaunchOptions

// initBrowser initializes the Playwright browser with retry logic
func initBrowser() error {
	slog.Info("Initializing Playwright for Letterboxd integration...")

	// Explicitly set the browser path to match what we set up in Dockerfile and entrypoint
	browserPath := os.Getenv("PLAYWRIGHT_BROWSERS_PATH")
	if browserPath == "" {
		browserPath = "/root/.cache/ms-playwright"
		os.Setenv("PLAYWRIGHT_BROWSERS_PATH", browserPath)
	}
	slog.Info("Using Playwright browsers path", slog.String("path", browserPath))
	
	// Verify browser path exists
	if _, err := os.Stat(browserPath); os.IsNotExist(err) {
		slog.Warn("Browser path does not exist, creating it", slog.String("path", browserPath))
		if err := os.MkdirAll(browserPath, 0755); err != nil {
			slog.Error("Failed to create browser path", 
				slog.String("path", browserPath), 
				slog.String("error", err.Error()))
		}
	}

	// Check for drivers before attempting to run
	driverCheck := exec.Command("find", browserPath, "-name", "*.jar", "-o", "-name", "*.exe")
	output, _ := driverCheck.Output()
	if len(output) == 0 {
		slog.Warn("No driver files found in browsers path, attempting to install")
		installCmd := exec.Command("playwright", "install")
		installCmd.Stdout = os.Stdout
		installCmd.Stderr = os.Stderr
		if err := installCmd.Run(); err != nil {
			slog.Error("Failed to install browsers", slog.String("error", err.Error()))
		}
	}

	// Attempt to run Playwright with retry logic
	var runErr error

	// Multiple attempts with different strategies
	for attempt := 1; attempt <= 3; attempt++ {
		slog.Info("Attempting to initialize Playwright", slog.Int("attempt", attempt))
		pw, runErr = playwright.Run()

		if runErr == nil {
			slog.Info("Playwright initialized successfully")
			break
		}

		slog.Error("Failed to initialize Playwright",
			slog.String("error", runErr.Error()),
			slog.Int("attempt", attempt))

		// Try different recovery strategies
		switch attempt {
		case 1:
			// Try using the go-specific playwright tool
			cmd := exec.Command("playwright", "install", "--with-deps")
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Run()
		case 2:
			// Try npm installation with specific version
			slog.Info("Trying NPM installation...")
			os.Chdir("/tmp")
			npmCmd := exec.Command("npm", "install", "playwright@1.49.1")
			npmCmd.Run()
			npxCmd := exec.Command("npx", "playwright@1.49.1", "install")
			npxCmd.Run()
			// Create explicit symlinks for the drivers
			exec.Command("mkdir", "-p", browserPath+"/firefox-1491").Run()
			exec.Command("cp", "-r", browserPath+"/firefox-*/*", browserPath+"/firefox-1491/").Run()
		}
	}

	// If all attempts failed, return error
	if runErr != nil {
		return runErr
	}

	// Configure browser launch options
	var headless = true
	browserLaunchOptions = playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
		Timeout:  playwright.Float(120000), // 120 seconds timeout for browser launch
		Args: []string{
			"--no-sandbox",
			"--disable-setuid-sandbox",
			"--disable-dev-shm-usage",
			"--disable-accelerated-2d-canvas",
			"--no-first-run",
			"--no-zygote",
			"--disable-gpu",
			"--disable-background-timer-throttling",
			"--disable-backgrounding-occluded-windows",
			"--disable-renderer-backgrounding",
			"--disable-features=TranslateUI",
			"--disable-ipc-flooding-protection",
			"--disable-extensions",
			"--disable-component-extensions-with-background-pages",
			"--disable-default-apps",
			"--mute-audio",
			"--no-default-browser-check",
			"--ignore-certificate-errors",
			"--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36",
		},
		IgnoreDefaultArgs: []string{"--enable-automation"},
	}

	return launchBrowser()
}

// launchBrowser launches the Firefox browser with the configured options
func launchBrowser() error {
	slog.Info("Launching Firefox browser...")
	var err error
	
	// Try multiple launch attempts with different configurations
	for attempt := 1; attempt <= 3; attempt++ {
		slog.Info("Attempting to launch Firefox browser", slog.Int("attempt", attempt))
		
		// Modify options for different attempts if needed
		if attempt > 1 {
			// Add more aggressive options for subsequent attempts
			browserLaunchOptions.Args = append(browserLaunchOptions.Args, 
				"--disable-features=site-per-process",
				"--disable-web-security",
				"--disable-site-isolation-trials",
				"--disable-features=IsolateOrigins,site-per-process")
		}
		
		// Try to launch browser
		browser, err = pw.Firefox.Launch(browserLaunchOptions)
		if err == nil {
			slog.Info("Firefox browser launched successfully", slog.Int("attempt", attempt))
			
			// Verify browser is connected
			if browser.IsConnected() {
				slog.Info("Browser connection verified")
				return nil
			} else {
				slog.Warn("Browser launched but not connected, will retry")
				// Close the disconnected browser
				try := func() {
					defer func() {
						if r := recover(); r != nil {
							slog.Warn("Recovered from panic when closing browser", slog.Any("recover", r))
						}
					}()
					browser.Close()
				}
				try()
			}
		} else {
			slog.Error("Failed to launch Firefox browser", 
				slog.String("error", err.Error()),
				slog.Int("attempt", attempt))
		}
		
		// Wait before retry
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	
	slog.Error("Failed to launch Firefox browser after multiple attempts")
	return err
}

// ensureBrowserAvailable checks if the browser is available and reconnects if needed
func ensureBrowserAvailable() bool {
	// Check if Playwright is initialized
	if pw == nil {
		slog.Error("Playwright is not initialized, attempting full initialization")
		if err := initBrowser(); err != nil {
			slog.Error("Failed to initialize Playwright", slog.String("error", err.Error()))
			return false
		}
		return true
	}

	// Check if browser is nil or closed
	if browser == nil {
		slog.Warn("Browser is nil, attempting to initialize")
		if err := launchBrowser(); err != nil {
			slog.Error("Failed to launch browser", slog.String("error", err.Error()))
			// Try full reinitialization as a last resort
			slog.Warn("Attempting full Playwright reinitialization")
			if err := initBrowser(); err != nil {
				slog.Error("Failed to reinitialize Playwright", slog.String("error", err.Error()))
				return false
			}
		}
		return browser != nil && browser.IsConnected()
	}

	// Check if browser is connected with a deeper validity check
	isConnected := false
	try := func() bool {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Panic when checking browser connection", slog.Any("recover", r))
			}
		}()
		
		// Basic connection check
		if !browser.IsConnected() {
			return false
		}
		
		// Try to access a browser property as a deeper validity test
		_ = browser.Contexts()
		return true
	}
	
	isConnected = try()
	
	if !isConnected {
		slog.Warn("Browser appears to be disconnected, attempting to reconnect")
		
		// Try to close the browser safely before reconnecting
		tryClose := func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("Recovered from panic when closing browser", slog.Any("recover", r))
				}
			}()
			browser.Close()
		}
		tryClose()
		
		// Set to nil to force recreation
		browser = nil
		
		// Try to relaunch
		if err := launchBrowser(); err != nil {
			slog.Error("Failed to reconnect browser", slog.String("error", err.Error()))
			
			// Try full reinitialization as a last resort
			slog.Warn("Attempting full Playwright reinitialization")
			if err := initBrowser(); err != nil {
				slog.Error("Failed to reinitialize Playwright", slog.String("error", err.Error()))
				return false
			}
		}
	}

	return browser != nil && browser.IsConnected()
}

func init() {
	// Initialize the browser with retry logic
	if err := initBrowser(); err != nil {
		panic("Failed to initialize Playwright after multiple attempts: " + err.Error())
	}
}

func NewUser(username string, password string, allowRelog bool) User {
	// Configure browser context with improved options
	var context, contextErr = browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1920,
			Height: 1080,
		},
		UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
		BypassCSP: playwright.Bool(true),  // Bypass Content-Security-Policy
		IgnoreHttpsErrors: playwright.Bool(true), // Ignore HTTPS errors
		ExtraHttpHeaders: map[string]string{
			"Accept-Language": "en-US,en;q=0.9",
		},
	})
	if contextErr != nil {
		panic(contextErr)
	}

	// Configure default timeout for context
	context.SetDefaultTimeout(90000) // 90 seconds

	return User{
		username,
		password,
		context,
		allowRelog,
	}
}

type User struct {
	username   string
	password   string
	context    playwright.BrowserContext
	allowRelog bool
}

func (l *User) newPage(url string) playwright.Page {
	// First ensure browser is available
	if !ensureBrowserAvailable() {
		slog.Error("Browser is not available",
			slog.String("username", l.username),
			slog.String("url", url))
		return nil
	}
	
	// Check if existing context is valid
	contextValid := false
	if l.context != nil {
		// Try to check if context is still valid
		try := func() bool {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("Recovered from panic when checking context", 
						slog.Any("recover", r),
						slog.String("username", l.username))
				}
			}()
			
			// Simple check if browser is connected
			if !browser.IsConnected() {
				return false
			}
			
			// Try to get browser info as a validity check
			pages := l.context.Pages()
			
			// Check if we have valid pages
			if len(pages) == 0 {
				slog.Warn("No pages found in context, may be invalid", 
					slog.String("username", l.username))
				return false
			}
			
			// Try to access a page property as a deeper validity check
			if len(pages) > 0 {
				// Try to get URL from first page as a validity test
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Panic when accessing page in context", 
							slog.Any("recover", r),
							slog.String("username", l.username))
					}
				}()
				
				// Just try to access a property - if context is invalid this will fail
				_ = pages[0].URL()
			}
			
			return true
		}
		
		contextValid = try()
	}
	
	// Close existing context if it's invalid
	if l.context != nil && !contextValid {
		slog.Info("Closing invalid browser context", slog.String("username", l.username))
		// Safely close the context
		try := func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Warn("Recovered from panic when closing context", 
						slog.Any("recover", r))
				}
			}()
			l.context.Close()
		}
		try()
		l.context = nil
	}

	// Create a new context if needed
	if l.context == nil {
		slog.Info("Creating new browser context", slog.String("username", l.username))
		context, err := browser.NewContext(playwright.BrowserNewContextOptions{
			Viewport: &playwright.Size{
				Width:  1920,
				Height: 1080,
			},
			UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
			BypassCSP: playwright.Bool(true),  // Bypass Content-Security-Policy
			IgnoreHttpsErrors: playwright.Bool(true), // Ignore HTTPS errors
			ExtraHttpHeaders: map[string]string{
				"Accept-Language": "en-US,en;q=0.9",
			},
		})
		if err != nil {
			slog.Error("Failed to create browser context",
				slog.String("error", err.Error()),
				slog.String("username", l.username))
			return nil
		}
		// Update the context in the User struct
		l.context = context
		// Configure default timeout for context
		l.context.SetDefaultTimeout(90000) // 90 seconds
	}
	
	// Now create the page with retry logic
	var page playwright.Page
	var pageErr error
	
	// Try up to 3 times to create a page
	for attempt := 1; attempt <= 3; attempt++ {
		page, pageErr = l.context.NewPage()
		if pageErr == nil && page != nil {
			break
		}
		
		slog.Warn("Failed to create page, retrying",
			slog.String("error", func() string {
				if pageErr != nil {
					return pageErr.Error()
				}
				return "unknown error (nil)"
			}()),
			slog.String("username", l.username),
			slog.Int("attempt", attempt))
		
		// Wait before retry
		time.Sleep(time.Duration(attempt) * time.Second)
		
		// If we've tried multiple times and still failing, recreate the context
		if attempt >= 2 && l.context != nil {
			slog.Info("Recreating browser context after failed page creation", 
				slog.String("username", l.username))
			
			// Safely close the context
			try := func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Warn("Recovered from panic when closing context", 
							slog.Any("recover", r))
					}
				}()
				l.context.Close()
			}
			try()
			
			// Create new context
			context, err := browser.NewContext(playwright.BrowserNewContextOptions{
				Viewport: &playwright.Size{
					Width:  1920,
					Height: 1080,
				},
				UserAgent: playwright.String("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
				BypassCSP: playwright.Bool(true),
				IgnoreHttpsErrors: playwright.Bool(true),
				ExtraHttpHeaders: map[string]string{
					"Accept-Language": "en-US,en;q=0.9",
				},
			})
			if err != nil {
				slog.Error("Failed to recreate browser context",
					slog.String("error", err.Error()),
					slog.String("username", l.username))
				continue
			}
			
			// Update context
			l.context = context
			l.context.SetDefaultTimeout(90000)
		}
	}
	
	if pageErr != nil || page == nil {
		slog.Error("Failed to create new page after multiple attempts",
			slog.String("error", func() string {
				if pageErr != nil {
					return pageErr.Error()
				}
				return "unknown error"
			}()),
			slog.String("username", l.username),
			slog.String("url", url))
		return nil
	}

	// Configure page for better reliability
	// Increase default timeout for all operations
	page.SetDefaultTimeout(90000) // 90 seconds

	// Add retry logic for page navigation
	var err error
	var success bool
	timeout := 90000.0 // 90 seconds timeout
	
	// Try up to 3 times with increasing timeouts
	for attempt := 1; attempt <= 3; attempt++ {
		slog.Info("Navigating to page", 
			slog.String("url", url), 
			slog.Int("attempt", attempt))
		
		// Add wait time between retries
		if attempt > 1 {
			slog.Info("Waiting before retry", slog.Int("seconds", attempt))
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		// Configure navigation options
		options := playwright.PageGotoOptions{
			Timeout: &timeout,
			WaitUntil: playwright.WaitUntilStateNetworkidle, // Wait until network is idle
		}

		// Attempt navigation
		response, err := page.Goto(url, options)
		
		// Check for success
		if err == nil && response != nil {
			status := response.Status()
			if status >= 200 && status < 400 {
				slog.Info("Page loaded successfully", 
					slog.String("url", url),
					slog.Int("status", status),
					slog.Int("attempt", attempt))
				success = true
				break
			} else {
				slog.Warn("Page returned error status", 
					slog.String("url", url),
					slog.Int("status", status))
			}
		} else if err != nil {
			slog.Warn("Navigation failed", 
				slog.String("url", url),
				slog.String("error", err.Error()),
				slog.Int("attempt", attempt))
		}
		
		// Increase timeout for next attempt
		timeout += 30000 // Add 30 seconds for each retry
	}

	if !success {
		slog.Error("All navigation attempts failed", 
			slog.String("url", url),
			slog.String("error", func() string {
				if err != nil {
					return err.Error()
				}
				return "unknown error"
			}()))
		
		// Take a screenshot of the failed navigation for debugging
		screenshotPath := "/tmp/letterboxd-navigation-failed.png"
		if _, err := page.Screenshot(playwright.PageScreenshotOptions{
			Path: playwright.String(screenshotPath),
		}); err == nil {
			slog.Info("Saved failed navigation screenshot", slog.String("path", screenshotPath))
		}
	}

	return page
}
