package api

import (
	"fmt"
	"log/slog"

	"emboxd/history"
	"emboxd/letterboxd"
	"emboxd/notification"

	"github.com/gin-gonic/gin"
)

type Api struct {
	router                               *gin.Engine
	notificationProcessorByEmbyUsername  map[string]*notification.Processor
	notificationProcessorByPlexUsername  map[string]*notification.Processor
	notificationProcessorByPlexAccountID map[string]*notification.Processor
	letterboxdWorkers                    map[string]*letterboxd.Worker
	eventHistory                         *history.Store
	metrics                              *Metrics
}

func New(
	notificationProcessorByEmbyUsername,
	notificationProcessorByPlexUsername,
	notificationProcessorByPlexAccountID map[string]*notification.Processor,
	letterboxdWorkers map[string]*letterboxd.Worker,
	historySize int,
) Api {
	gin.SetMode(gin.ReleaseMode)

	// Create metrics
	metrics := NewMetrics()

	// Create router with our custom middleware instead of default
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(LoggingMiddleware())
	router.Use(MetricsMiddleware(metrics))

	return Api{
		router:                               router,
		notificationProcessorByEmbyUsername:  notificationProcessorByEmbyUsername,
		notificationProcessorByPlexUsername:  notificationProcessorByPlexUsername,
		notificationProcessorByPlexAccountID: notificationProcessorByPlexAccountID,
		letterboxdWorkers:                    letterboxdWorkers,
		eventHistory:                         history.NewStore(historySize),
		metrics:                              metrics,
	}
}

func (a *Api) getRoot(context *gin.Context) {
	context.String(200, "Welcome to EmBoxd!")
}

func (a *Api) setupRoutes() {
	a.setupEmbyRoutes()
	a.setupPlexRoutes()
	a.setupHealthRoutes()
	a.setupEventsRoutes()
	a.setupMetricsRoutes()

	a.router.GET("/", a.getRoot)
}

func (a *Api) Handler() *gin.Engine {
	a.setupRoutes()
	return a.router
}

func (a *Api) Run(port int) {
	a.setupRoutes()

	slog.Info("Starting Gin Server")
	a.router.Run(fmt.Sprintf(":%d", port))
}
