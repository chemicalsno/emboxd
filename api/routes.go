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
	router                              *gin.Engine
	notificationProcessorByEmbyUsername map[string]*notification.Processor
	notificationProcessorByPlexUsername map[string]*notification.Processor
	notificationProcessorByPlexAccountID map[string]*notification.Processor
	letterboxdWorkers                   map[string]*letterboxd.Worker
	eventHistory                        *history.Store
}

func New(
	notificationProcessorByEmbyUsername, 
	notificationProcessorByPlexUsername, 
	notificationProcessorByPlexAccountID map[string]*notification.Processor,
	letterboxdWorkers map[string]*letterboxd.Worker,
	historySize int,
) Api {
	gin.SetMode(gin.ReleaseMode)
	return Api{
		router:                              gin.Default(),
		notificationProcessorByEmbyUsername: notificationProcessorByEmbyUsername,
		notificationProcessorByPlexUsername: notificationProcessorByPlexUsername,
		notificationProcessorByPlexAccountID: notificationProcessorByPlexAccountID,
		letterboxdWorkers:                   letterboxdWorkers,
		eventHistory:                        history.NewStore(historySize),
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
