package api

import (
	"bytes"
	"io"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// responseWriter is a custom writer to capture the response
type responseWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

// Write captures the response body
func (w *responseWriter) Write(b []byte) (int, error) {
	w.body.Write(b)
	return w.ResponseWriter.Write(b)
}

// LoggingMiddleware logs requests and responses
func LoggingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Log the request
		slog.Debug("Received API request",
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.String("query", raw),
			slog.String("ip", c.ClientIP()),
			slog.String("user-agent", c.Request.UserAgent()),
		)

		// Skip logging request body for certain endpoints
		if !shouldSkipRequestBodyLogging(path) {
			// Read request body
			var requestBody []byte
			if c.Request.Body != nil {
				requestBody, _ = io.ReadAll(c.Request.Body)
				// Restore the body
				c.Request.Body = io.NopCloser(bytes.NewBuffer(requestBody))

				// Log request body if present and not too large
				if len(requestBody) > 0 && len(requestBody) < 10000 {
					slog.Debug("Request body",
						slog.String("path", path),
						slog.String("body", string(requestBody)),
					)
				}
			}
		}

		// Create custom writer
		w := &responseWriter{ResponseWriter: c.Writer, body: &bytes.Buffer{}}
		c.Writer = w

		// Process request
		c.Next()

		// After request
		latency := time.Since(start)
		statusCode := c.Writer.Status()
		responseSize := c.Writer.Size()

		// Log basic info for all requests
		logLevel := slog.LevelInfo
		if statusCode >= 400 {
			logLevel = slog.LevelWarn
		}
		if statusCode >= 500 {
			logLevel = slog.LevelError
		}

		slog.Log(c.Request.Context(), logLevel, "API request completed",
			slog.String("method", c.Request.Method),
			slog.String("path", path),
			slog.Int("status", statusCode),
			slog.Int("size", responseSize),
			slog.Duration("latency", latency),
			slog.String("ip", c.ClientIP()),
		)

		// Log response body for errors or if in debug mode
		if !shouldSkipResponseBodyLogging(path) && (statusCode >= 400 || slog.Default().Enabled(c.Request.Context(), slog.LevelDebug)) {
			responseBody := w.body.String()
			if len(responseBody) > 0 && len(responseBody) < 10000 {
				slog.Log(c.Request.Context(), logLevel, "Response body",
					slog.String("path", path),
					slog.String("body", responseBody),
				)
			}
		}

		// Add error logs for failed requests
		if len(c.Errors) > 0 {
			slog.Error("Request errors",
				slog.String("path", path),
				slog.String("errors", c.Errors.String()),
			)
		}
	}
}

// shouldSkipRequestBodyLogging returns true if request body logging should be skipped
func shouldSkipRequestBodyLogging(path string) bool {
	// Skip health checks
	if path == "/health" {
		return true
	}
	return false
}

// shouldSkipResponseBodyLogging returns true if response body logging should be skipped
func shouldSkipResponseBodyLogging(path string) bool {
	// Skip health checks
	if path == "/health" {
		return true
	}
	return false
}
