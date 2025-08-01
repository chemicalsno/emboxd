package letterboxd

import (
	"log/slog"
	"time"
)

const _EVENT_BUFFER_SIZE int = 10

type Action int

const (
	FilmUnwatched Action = iota
	FilmWatched
	FilmLogged
)

type Event struct {
	ImdbId string
	Action Action
	Time   time.Time
}

type Worker struct {
	debouncer
	user    User
	channel chan Event
}

func NewWorker(username string, password string) Worker {
	var channel = make(chan Event, _EVENT_BUFFER_SIZE)
	return Worker{
		debouncer: newDebouncer(
			channel,
		),
		user: NewUser(
			username,
			password,
		),
		channel: channel,
	}
}

func (w *Worker) HandleEvent(event Event) {
	w.debounce(event)
}

func (w *Worker) Start() {
	go w.run()
}

func (w *Worker) run() {
	// Initial login
	err := w.user.Login()
	if err != nil {
		slog.Error("Failed to login during worker initialization",
			slog.String("username", w.user.username),
			slog.String("error", err.Error()))
	}

	for {
		var event = <-w.channel

		// Process each event with proper error handling
		var actionStr string
		var err error

		switch event.Action {
		case FilmWatched, FilmUnwatched:
			actionStr = "mark film as " + map[bool]string{true: "watched", false: "unwatched"}[event.Action == FilmWatched]
			err = w.user.SetFilmWatched(event.ImdbId, event.Action == FilmWatched)
		case FilmLogged:
			actionStr = "log film as watched"
			err = w.user.LogFilmWatched(event.ImdbId)
		default:
			slog.Error("Unknown event action",
				slog.Int("action", int(event.Action)),
				slog.String("imdbId", event.ImdbId))
			continue
		}

		if err != nil {
			slog.Error("Failed to process event",
				slog.String("action", actionStr),
				slog.String("imdbId", event.ImdbId),
				slog.String("error", err.Error()),
				slog.Time("eventTime", event.Time))
		} else {
			slog.Info("Successfully processed event",
				slog.String("action", actionStr),
				slog.String("imdbId", event.ImdbId),
				slog.Time("eventTime", event.Time))
		}
	}
}
