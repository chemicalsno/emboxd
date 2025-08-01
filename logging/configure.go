package logging

import (
	"log/slog"
	"os"
	"path/filepath"
)

// Configure sets up the logging system
// For backward compatibility, it calls ConfigureEnhanced with default settings
func Configure(verbose bool) {
	// Create default config
	config := DefaultLogConfig(verbose)

	// Use logs directory in current directory if it exists
	logsDir := filepath.Join(".", "logs")
	if _, err := os.Stat(logsDir); !os.IsNotExist(err) {
		config.LogDirectory = logsDir
	} else {
		// Otherwise just log to stdout
		config.LogDirectory = ""
	}

	// Use simple handler if enhanced configuration fails
	if err := ConfigureEnhanced(config); err != nil {
		// Fall back to basic handler
		var handler = _Handler{
			level: config.Level,
		}
		var logger = slog.New(handler)
		slog.SetDefault(logger)
		slog.Error("Failed to set up enhanced logging, falling back to basic", 
			slog.String("error", err.Error()))
	}
}
