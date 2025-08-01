package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"emboxd/api"
	"emboxd/config"
	"emboxd/letterboxd"
	"emboxd/logging"
	"emboxd/notification"
)

func main() {
	var verbose bool
	var configFilename string
	var historySize int
	var logDir string
	var logJson bool
	var port string

	// Command-line flags
	flag.BoolVar(&verbose, "v", false, "Enable debug logging")
	flag.BoolVar(&verbose, "verbose", false, "Enable debug logging")
	flag.StringVar(&configFilename, "c", "config/config.yaml", "Path to configuration file")
	flag.StringVar(&configFilename, "config", "config/config.yaml", "Path to configuration file")
	flag.IntVar(&historySize, "history-size", 100, "Maximum number of events to keep in history")
	flag.StringVar(&logDir, "log-dir", "", "Directory for log files (empty for stdout only)")
	flag.BoolVar(&logJson, "log-json", false, "Output logs in JSON format")
	flag.StringVar(&port, "port", "9001", "Port to listen on")
	flag.Parse()

	// Environment variable overrides
	if envSize := os.Getenv("HISTORY_SIZE"); envSize != "" {
		if size, err := strconv.Atoi(envSize); err == nil && size > 0 {
			historySize = size
		}
	}

	if envLogDir := os.Getenv("LOG_DIR"); envLogDir != "" {
		logDir = envLogDir
	}

	if envLogJson := os.Getenv("LOG_JSON"); envLogJson != "" {
		logJson = envLogJson == "true" || envLogJson == "1" || envLogJson == "yes"
	}

	if envLogLevel := os.Getenv("LOG_LEVEL"); envLogLevel == "debug" {
		verbose = true
	}

	if envPort := os.Getenv("PORT"); envPort != "" {
		port = envPort
		fmt.Printf("Using PORT from environment: %s\n", port)
	} else {
		fmt.Printf("Using default port: %s\n", port)
	}

	// Configure enhanced logging
	logConfig := logging.DefaultLogConfig(verbose)
	if logDir != "" {
		logConfig.LogDirectory = logDir
	}
	logConfig.EnableJSON = logJson

	if err := logging.ConfigureEnhanced(logConfig); err != nil {
		// Fall back to basic logging
		logging.Configure(verbose)
	}
	var conf = config.Load(configFilename)

	var notificationProcessorByEmbyUsername = make(map[string]*notification.Processor, len(conf.Users))
	var notificationProcessorByPlexUsername = make(map[string]*notification.Processor, len(conf.Users))
	var notificationProcessorByPlexAccountID = make(map[string]*notification.Processor, len(conf.Users))
	var letterboxdWorkers = make(map[string]*letterboxd.Worker, len(conf.Users))
	for _, user := range conf.Users {
		var letterboxdWorker, workerExists = letterboxdWorkers[user.Letterboxd.Username]
		if !workerExists {
			var worker = letterboxd.NewWorker(user.Letterboxd.Username, user.Letterboxd.Password)
			worker.Start()
			letterboxdWorker = &worker
			letterboxdWorkers[user.Letterboxd.Username] = letterboxdWorker
		}

		var notificationProcessor = notification.NewProcessor(letterboxdWorker.HandleEvent)
		if user.Emby.Username != "" {
			notificationProcessorByEmbyUsername[user.Emby.Username] = &notificationProcessor
		}
		if user.Plex.Username != "" {
			notificationProcessorByPlexUsername[user.Plex.Username] = &notificationProcessor
		}
		if user.Plex.ID != "" {
			notificationProcessorByPlexAccountID[user.Plex.ID] = &notificationProcessor
		}
	}

	var app = api.New(
		notificationProcessorByEmbyUsername,
		notificationProcessorByPlexUsername,
		notificationProcessorByPlexAccountID,
		letterboxdWorkers,
		historySize,
	)

	// Use graceful shutdown server
	handler := app.Handler()
	server := NewGracefulServer(":"+port, handler)

	if err := server.Start(); err != nil {
		slog.Error("Server error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}
