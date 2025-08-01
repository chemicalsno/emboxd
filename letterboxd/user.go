package letterboxd

import (
	"log/slog"

	"github.com/playwright-community/playwright-go"
)

var browser playwright.Browser

func init() {
	slog.Info("Initializing Playwright for Letterboxd integration...")

	// Attempt to run Playwright
	var pw, runErr = playwright.Run()
	if runErr != nil {
		slog.Error("Failed to initialize Playwright driver",
			slog.String("error", runErr.Error()))

		// More helpful panic message
		if runErr.Error() == "please install the driver (v1.49.1) first: %!w(<nil>)" {
			panic("Playwright driver v1.49.1 not found. Please run 'playwright install' manually or check the Playwright browsers path environment variable.")
		}
		panic(runErr)
	}

	var headless = true
	var launchOptions = playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
		Timeout:  playwright.Float(60000), // 60 seconds timeout
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
		// Instead of panicking, we return a nil page, and callers will need to check
		// But since this is a major architectural change, we'll keep the panic for now
		panic(pageErr)
	}

	if _, err := page.Goto(url); err != nil {
		// Acceptable due to ad loading or other non-critical resources
		slog.Warn("Page took too long to load",
			slog.String("url", url),
			slog.String("error", err.Error()))
	}

	return page
}
