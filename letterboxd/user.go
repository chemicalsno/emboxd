package letterboxd

import (
	"fmt"
	"log/slog"
)

import "github.com/playwright-community/playwright-go"

var browser playwright.Browser

func init() {
	var pw, runErr = playwright.Run()
	if runErr != nil {
		panic(runErr)
	}

	var headless = true
	var launchOptions = playwright.BrowserTypeLaunchOptions{Headless: &headless}
	if b, err := pw.Firefox.Launch(launchOptions); err == nil {
		browser = b
	} else {
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
