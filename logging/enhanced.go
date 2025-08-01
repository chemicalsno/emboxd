package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// LogConfig holds configuration for logging
type LogConfig struct {
	Level        slog.Level
	EnableJSON   bool
	LogDirectory string
	FileName     string
	MaxSize      int64 // in MB
	MaxAge       int   // in days
	MaxBackups   int
	LocalTime    bool
	Compress     bool
}

// DefaultLogConfig returns default log configuration
func DefaultLogConfig(verbose bool) LogConfig {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	return LogConfig{
		Level:        level,
		EnableJSON:   false,
		LogDirectory: "logs",
		FileName:     "emboxd.log",
		MaxSize:      100,  // 100MB
		MaxAge:       30,   // 30 days
		MaxBackups:   5,
		LocalTime:    true,
		Compress:     true,
	}
}

// RotatingFile handles log file rotation
type RotatingFile struct {
	sync.Mutex
	filename   string
	maxSize    int64
	size       int64
	file       *os.File
	maxBackups int
	maxAge     int
}

// NewRotatingFile creates a new rotating file logger
func NewRotatingFile(filename string, maxSizeMB int64, maxBackups, maxAge int) (*RotatingFile, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	rf := &RotatingFile{
		filename:   filename,
		maxSize:    maxSizeMB * 1024 * 1024,
		maxBackups: maxBackups,
		maxAge:     maxAge,
	}

	if err := rf.openFile(); err != nil {
		return nil, err
	}

	return rf, nil
}

func (r *RotatingFile) openFile() error {
	// Get file info to determine size
	info, err := os.Stat(r.filename)
	if err == nil {
		r.size = info.Size()
	}

	// Open/create the file
	f, err := os.OpenFile(r.filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	r.file = f
	return nil
}

// Write implements io.Writer
func (r *RotatingFile) Write(p []byte) (n int, err error) {
	r.Lock()
	defer r.Unlock()

	writeLen := int64(len(p))
	if writeLen+r.size > r.maxSize {
		if err := r.rotate(); err != nil {
			return 0, err
		}
	}

	n, err = r.file.Write(p)
	r.size += int64(n)
	return n, err
}

// rotate rotates the file
func (r *RotatingFile) rotate() error {
	if r.file != nil {
		r.file.Close()
		r.file = nil
	}

	// Rename current log file
	backupName := fmt.Sprintf("%s.%s", r.filename, time.Now().Format("2006-01-02-15-04-05"))
	if err := os.Rename(r.filename, backupName); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to rename log file: %w", err)
	}

	// Clean up old backups
	r.cleanupOldBackups()

	// Open a new file
	if err := r.openFile(); err != nil {
		return fmt.Errorf("failed to open new log file: %w", err)
	}
	
	r.size = 0
	return nil
}

// cleanupOldBackups removes old backup files
func (r *RotatingFile) cleanupOldBackups() {
	// Get the directory and file pattern
	dir := filepath.Dir(r.filename)
	pattern := filepath.Base(r.filename) + ".*"
	
	// Find all backup files
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return
	}

	// Keep track of backup files with their modification times
	type backupFile struct {
		path    string
		modTime time.Time
	}
	var backups []backupFile

	// Get info for each backup file
	for _, path := range matches {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		backups = append(backups, backupFile{path: path, modTime: info.ModTime()})
	}

	// Sort by modification time (newest first)
	// This is a simple bubble sort - fine for small number of files
	for i := 0; i < len(backups)-1; i++ {
		for j := i + 1; j < len(backups); j++ {
			if backups[i].modTime.Before(backups[j].modTime) {
				backups[i], backups[j] = backups[j], backups[i]
			}
		}
	}

	// Remove files that are too old or exceed the max number of backups
	cutoffTime := time.Now().AddDate(0, 0, -r.maxAge)
	for i, backup := range backups {
		if i >= r.maxBackups || (r.maxAge > 0 && backup.modTime.Before(cutoffTime)) {
			os.Remove(backup.path)
		}
	}
}

// Close closes the file
func (r *RotatingFile) Close() error {
	r.Lock()
	defer r.Unlock()

	if r.file != nil {
		err := r.file.Close()
		r.file = nil
		return err
	}
	return nil
}

// EnhancedHandler extends the basic handler with more features
type EnhancedHandler struct {
	Level     slog.Level
	Writer    io.Writer
	UseJSON   bool
	AddSource bool
	Attrs     []slog.Attr
	Groups    []string
}

// Enabled implements slog.Handler
func (h *EnhancedHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.Level
}

// Handle implements slog.Handler
func (h *EnhancedHandler) Handle(ctx context.Context, record slog.Record) error {
	// Get call frame information
	var frame runtime.Frame
	if h.AddSource && record.PC != 0 {
		frames := runtime.CallersFrames([]uintptr{record.PC})
		frame, _ = frames.Next()
	}

	timestamp := record.Time.Format("2006-01-02T15:04:05.000Z07:00")
	level := record.Level.String()
	message := record.Message
	
	var output string
	if h.UseJSON {
		// Build JSON output with all attributes
		attrs := make(map[string]interface{})
		attrs["timestamp"] = timestamp
		attrs["level"] = level
		attrs["message"] = message
		
		if h.AddSource && record.PC != 0 {
			attrs["file"] = frame.File
			attrs["line"] = frame.Line
			attrs["function"] = frame.Function
		}

		// Add handler attributes
		for _, attr := range h.Attrs {
			attrs[attr.Key] = attr.Value.String()
		}

		// Add record attributes
		record.Attrs(func(attr slog.Attr) bool {
			var key = attr.Key
			if len(h.Groups) > 0 {
				key = strings.Join(h.Groups, ".") + "." + key
			}
			attrs[key] = attr.Value.String()
			return true
		})

		// Create JSON string
		jsonStr := "{"
		i := 0
		for k, v := range attrs {
			if i > 0 {
				jsonStr += ", "
			}
			jsonStr += fmt.Sprintf(`"%s": "%v"`, k, v)
			i++
		}
		jsonStr += "}\n"
		output = jsonStr
	} else {
		// Build structured text output
		var builder strings.Builder
		builder.WriteString(fmt.Sprintf("[%s] [%s] %s", timestamp, level, message))

		// Add source info
		if h.AddSource && record.PC != 0 {
			shortFile := frame.File
			if lastSlash := strings.LastIndexByte(shortFile, '/'); lastSlash >= 0 {
				shortFile = shortFile[lastSlash+1:]
			}
			builder.WriteString(fmt.Sprintf(" (%s:%d)", shortFile, frame.Line))
		}

		// Add all attributes
		record.Attrs(func(attr slog.Attr) bool {
			builder.WriteString(" ")
			var prefix string
			if len(h.Groups) > 0 {
				prefix = strings.Join(h.Groups, ".") + "."
			}
			builder.WriteString(prefix + attr.String())
			return true
		})
		builder.WriteString("\n")
		output = builder.String()
	}

	_, err := h.Writer.Write([]byte(output))
	return err
}

// WithAttrs implements slog.Handler
func (h *EnhancedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newHandler := *h
	newHandler.Attrs = append(h.Attrs, attrs...)
	return &newHandler
}

// WithGroup implements slog.Handler
func (h *EnhancedHandler) WithGroup(name string) slog.Handler {
	newHandler := *h
	newHandler.Groups = append(h.Groups, name)
	return &newHandler
}

// ConfigureEnhanced sets up enhanced logging with file rotation
func ConfigureEnhanced(config LogConfig) error {
	var writer io.Writer = os.Stdout

	// Set up file logging if directory is specified
	if config.LogDirectory != "" {
		logPath := filepath.Join(config.LogDirectory, config.FileName)
		rotator, err := NewRotatingFile(logPath, config.MaxSize, config.MaxBackups, config.MaxAge)
		if err != nil {
			return fmt.Errorf("failed to create log file: %w", err)
		}
		
		// Use both stdout and file for logging
		writer = io.MultiWriter(os.Stdout, rotator)
	}

	handler := &EnhancedHandler{
		Level:     config.Level,
		Writer:    writer,
		UseJSON:   config.EnableJSON,
		AddSource: true,
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)

	logger.Info("Logging system initialized",
		slog.String("level", config.Level.String()),
		slog.Bool("json", config.EnableJSON),
		slog.String("directory", config.LogDirectory),
		slog.String("file", config.FileName))

	return nil
}