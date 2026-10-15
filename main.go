package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"homeoffice-tracker/internal/store"
	"homeoffice-tracker/internal/web"
)

func main() {
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "/data/homeoffice.db"
	}

	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open database %s: %v", dbPath, err)
	}
	defer s.Close()

	addr := ":8080"
	log.Printf("listening on %s, database %s", addr, dbPath)
	log.Fatal(http.ListenAndServe(addr, web.NewHandler(s, time.Now)))
}
