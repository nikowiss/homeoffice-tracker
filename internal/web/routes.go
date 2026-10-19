package web

import (
	"context"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"homeoffice-tracker/internal/civil"
	"homeoffice-tracker/internal/store"
)

//go:embed templates/*.html templates/style.css
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var pageTemplate = template.Must(template.ParseFS(templateFS, "templates/index.html"))

type ReminderSetup struct {
	PublicKey string
	SendNow   func(context.Context) error
}

type server struct {
	store     *store.Store
	now       func() time.Time
	reminders ReminderSetup
}

func NewHandler(s *store.Store, now func() time.Time, reminders ReminderSetup) http.Handler {
	srv := &server{store: s, now: now, reminders: reminders}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", srv.index)
	mux.HandleFunc("POST /today", srv.markToday)
	mux.HandleFunc("POST /today/delete", srv.unmarkToday)
	mux.HandleFunc("POST /selection", srv.confirmSelection)
	mux.HandleFunc("POST /days/{date}", srv.markTodayCell)
	mux.HandleFunc("POST /days/{date}/delete", srv.unmarkTodayCell)
	mux.HandleFunc("GET /export/{year}", srv.export)
	mux.HandleFunc("POST /reminder/subscriptions", srv.subscribe)
	mux.HandleFunc("POST /reminder/subscriptions/delete", srv.unsubscribe)
	mux.HandleFunc("POST /reminder/test", srv.sendTestReminder)
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, templateFS, "templates/style.css")
	})
	for _, name := range []string{"favicon.ico", "favicon.svg", "apple-touch-icon.png", "sw.js", "reminder.js"} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, staticFS, "static/"+name)
		})
	}
	return http.NewCrossOriginProtection().Handler(mux)
}

func serverError(w http.ResponseWriter, r *http.Request, err error) {
	slog.Error("request failed", "method", r.Method, "path", r.URL.Path, "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

func (s *server) today() civil.Date {
	return civil.Today(s.now())
}
