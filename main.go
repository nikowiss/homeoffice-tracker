package main

import (
	"context"
	"fmt"
	"log/slog"
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

type config struct {
	addr         string
	dbPath       string
	reminderTime reminder.TimeOfDay
}

func loadConfig(getenv func(string) string) (config, error) {
	cfg := config{
		addr:         ":8080",
		dbPath:       "/data/homeoffice.db",
		reminderTime: reminder.TimeOfDay{Hour: 9},
	}
	if raw := getenv("DB_PATH"); raw != "" {
		cfg.dbPath = raw
	}
	if raw := getenv("REMINDER_TIME"); raw != "" {
		at, err := reminder.ParseTimeOfDay(raw)
		if err != nil {
			return config{}, fmt.Errorf("REMINDER_TIME: %w", err)
		}
		cfg.reminderTime = at
	}
	return cfg, nil
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Getenv)
	stop()
	if err != nil {
		slog.Error("homeoffice-tracker stopped", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, getenv func(string) string) error {
	cfg, err := loadConfig(getenv)
	if err != nil {
		return err
	}

	s, err := store.Open(ctx, cfg.dbPath)
	if err != nil {
		return fmt.Errorf("open database %s: %w", cfg.dbPath, err)
	}
	defer s.Close()

	keys, err := s.VAPIDKeys(ctx, webpush.GenerateVAPIDKeys)
	if err != nil {
		return fmt.Errorf("load VAPID keys: %w", err)
	}

	var background sync.WaitGroup
	defer background.Wait()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	reminders := reminder.New(s, reminder.WebPushSender{Keys: keys}, cfg.reminderTime, time.Now)
	background.Go(func() { reminders.Run(ctx) })

	server := &http.Server{
		Addr:              cfg.addr,
		Handler:           web.NewHandler(s, time.Now, web.ReminderSetup{PublicKey: keys.Public, SendNow: reminders.SendNow}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	slog.Info("homeoffice-tracker started",
		"version", version, "addr", cfg.addr, "db", cfg.dbPath, "reminder_time", cfg.reminderTime)

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancelShutdown := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shut down server: %w", err)
	}
	return nil
}
