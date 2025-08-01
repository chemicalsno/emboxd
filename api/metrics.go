package api

import (
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Metrics stores application metrics
type Metrics struct {
	mu                  sync.RWMutex
	startTime           time.Time
	requestCount        int64
	successfulRequests  int64
	failedRequests      int64
	webhookCount        map[string]int64 // Count by source (plex, emby)
	processingTime      map[string]int64 // Total processing time in ms by endpoint
	requestCountByPath  map[string]int64
	lastProcessingTimes []int64 // Last 100 processing times for histogram
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		startTime:           time.Now(),
		webhookCount:        make(map[string]int64),
		processingTime:      make(map[string]int64),
		requestCountByPath:  make(map[string]int64),
		lastProcessingTimes: make([]int64, 0, 100),
	}
}

// TrackRequest tracks a request
func (m *Metrics) TrackRequest(path string, status int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.requestCount++
	m.requestCountByPath[path]++

	durationMs := duration.Milliseconds()
	m.processingTime[path] += durationMs

	// Keep last 100 processing times
	if len(m.lastProcessingTimes) >= 100 {
		m.lastProcessingTimes = m.lastProcessingTimes[1:100]
	}
	m.lastProcessingTimes = append(m.lastProcessingTimes, durationMs)

	if status >= 200 && status < 400 {
		m.successfulRequests++
	} else {
		m.failedRequests++
	}
}

// TrackWebhook tracks a webhook
func (m *Metrics) TrackWebhook(source string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.webhookCount[source]++
}

// MetricsData holds the metrics data for the response
type MetricsData struct {
	Uptime             string            `json:"uptime"`
	RequestCount       int64             `json:"request_count"`
	SuccessfulRequests int64             `json:"successful_requests"`
	FailedRequests     int64             `json:"failed_requests"`
	WebhookCount       map[string]int64  `json:"webhook_count"`
	RequestsByPath     map[string]int64  `json:"requests_by_path"`
	AverageTimeByPath  map[string]int64  `json:"avg_time_by_path_ms"`
	MemoryStats        map[string]uint64 `json:"memory_stats"`
}

// GetMetricsData returns the current metrics
func (m *Metrics) GetMetricsData() MetricsData {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Calculate average processing time by path
	avgTimeByPath := make(map[string]int64)
	for path, totalTime := range m.processingTime {
		count := m.requestCountByPath[path]
		if count > 0 {
			avgTimeByPath[path] = totalTime / count
		}
	}

	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	memory := map[string]uint64{
		"alloc":       memStats.Alloc,
		"total_alloc": memStats.TotalAlloc,
		"sys":         memStats.Sys,
		"heap_alloc":  memStats.HeapAlloc,
		"heap_sys":    memStats.HeapSys,
		"num_gc":      uint64(memStats.NumGC),
	}

	return MetricsData{
		Uptime:             time.Since(m.startTime).Round(time.Second).String(),
		RequestCount:       m.requestCount,
		SuccessfulRequests: m.successfulRequests,
		FailedRequests:     m.failedRequests,
		WebhookCount:       m.webhookCount,
		RequestsByPath:     m.requestCountByPath,
		AverageTimeByPath:  avgTimeByPath,
		MemoryStats:        memory,
	}
}

// MetricsMiddleware adds request metrics
func MetricsMiddleware(metrics *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Process request
		c.Next()

		// After request
		duration := time.Since(start)
		status := c.Writer.Status()

		// Track metrics
		metrics.TrackRequest(c.Request.URL.Path, status, duration)
	}
}

// setupMetricsRoutes sets up metrics endpoints
func (a *Api) setupMetricsRoutes() {
	a.router.GET("/metrics", func(c *gin.Context) {
		c.JSON(http.StatusOK, a.metrics.GetMetricsData())
	})
}
