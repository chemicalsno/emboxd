package letterboxd

import (
	"log/slog"
	"os"
	"os/exec"

	"github.com/playwright-community/playwright-go"
)

var browser playwright.Browser

func init() {
	slog.Info("Initializing Playwright for Letterboxd integration...")

	// Explicitly set the browser path to match what we set up in Dockerfile and entrypoint
	browserPath := os.Getenv("PLAYWRIGHT_BROWSERS_PATH")
	if browserPath == "" {
		browserPath = "/root/.cache/ms-playwright"
		os.Setenv("PLAYWRIGHT_BROWSERS_PATH", browserPath)
	}
	slog.Info("Using Playwright browsers path", slog.String("path", browserPath))

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
	var pw *playwright.Playwright
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

	// If all attempts failed, we have to panic
	if runErr != nil {
		panic("Failed to initialize Playwright after multiple attempts: " + runErr.Error())
	}

	var headless = true
	var launchOptions = playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
		Timeout:  playwright.Float(60000), // 60 seconds timeout
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
		},
	}

	slog.Info("Launching Firefox browser...")
	if b, err := pw.Firefox.Launch(launchOptions); err == nil {
		browser = b
		slog.Info("Firefox browser launched successfully")
	} else {
		slog.Error("Failed to launch Firefox browser",
			slog.String("error", err.Error()))
		panic(err)
	}
}

func NewUser(username string, password string) User {
	var context, contextErr = browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{
			Width:  1920,
			Height: 1080,
		},
	})
	if contextErr != nil {
		panic(contextErr)
	}

	return User{
		username,
		password,
		context,
	}
}

type User struct {
	username string
	password string
	context  playwright.BrowserContext
}

func (l User) newPage(url string) playwright.Page {
	var page, pageErr = l.context.NewPage()
	if pageErr != nil {
		slog.Error("Failed to create new page",
			slog.String("error", pageErr.Error()),
			slog.String("username", l.username),
			slog.String("url", url))
		// Return nil instead of panicking to prevent crash loops
		return nil
	}

	if _, err := page.Goto(url); err != nil {
		// Acceptable due to ad loading or other non-critical resources
		slog.Warn("Page took too long to load",
			slog.String("url", url),
			slog.String("error", err.Error()))
	}

	return page
}
