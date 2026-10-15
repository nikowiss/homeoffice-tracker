package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/SherClockHolmes/webpush-go"

	"homeoffice-tracker/internal/reminder"
	"homeoffice-tracker/internal/store"
	"homeoffice-tracker/internal/web"
)

var version = "dev"

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/homeoffice.db"
	}
	reminderTime, err := reminder.ParseTimeOfDay(os.Getenv("REMINDER_TIME"))
	if err != nil {
		log.Fatal(err)
	}

	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open database %s: %v", dbPath, err)
	}
	defer s.Close()

	keys, err := s.VAPIDKeys(webpush.GenerateVAPIDKeys)
	if err != nil {
		log.Fatalf("load VAPID keys: %v", err)
	}
	reminders := reminder.New(s, reminder.WebPushSender{Keys: keys}, reminderTime, time.Now)
	go reminders.Run(context.Background())

	addr := ":8080"
	log.Printf("homeoffice-tracker %s listening on %s, database %s, reminder at %02d:%02d",
		version, addr, dbPath, reminderTime.Hour, reminderTime.Minute)
	handler := web.NewHandler(s, time.Now, web.ReminderSetup{PublicKey: keys.Public, SendNow: reminders.SendNow})
	log.Fatal(http.ListenAndServe(addr, handler))
}
