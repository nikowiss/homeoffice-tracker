package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"homeoffice-tracker/internal/reminder"
	"homeoffice-tracker/internal/store"
	"homeoffice-tracker/internal/web"
)

const shutdownTimeout = 10 * time.Second

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context) error {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/homeoffice.db"
	}
	reminderTime, err := reminder.ParseTimeOfDay(os.Getenv("REMINDER_TIME"))
	if err != nil {
		return err
	}

	s, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open database %s: %w", dbPath, err)
	}
	defer s.Close()

	keys, err := s.VAPIDKeys(webpush.GenerateVAPIDKeys)
	if err != nil {
		return fmt.Errorf("load VAPID keys: %w", err)
	}

	var background sync.WaitGroup
	defer background.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	reminders := reminder.New(s, reminder.WebPushSender{Keys: keys}, reminderTime, time.Now)
	background.Go(func() { reminders.Run(ctx) })

	server := &http.Server{
		Addr:              ":8080",
		Handler:           web.NewHandler(s, time.Now, web.ReminderSetup{PublicKey: keys.Public, SendNow: reminders.SendNow}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	log.Printf("homeoffice-tracker %s listening on %s, database %s, reminder at %02d:%02d",
		version, server.Addr, dbPath, reminderTime.Hour, reminderTime.Minute)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	log.Print("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down server: %w", err)
	}
	return nil
}
