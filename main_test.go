package main

import (
	"testing"

	"homeoffice-tracker/internal/reminder"
)

func env(values map[string]string) func(string) string {
	return func(key string) string { return values[key] }
}

func TestLoadConfigDefaults(t *testing.T) {
	got, err := loadConfig(env(nil))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	want := config{addr: ":8080", dbPath: "/data/homeoffice.db", reminderTime: reminder.TimeOfDay{Hour: 9}}
	if got != want {
		t.Errorf("loadConfig = %+v, want %+v", got, want)
	}
}

func TestLoadConfigFromEnvironment(t *testing.T) {
	got, err := loadConfig(env(map[string]string{"DB_PATH": "/tmp/test.db", "REMINDER_TIME": "08:30"}))
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if got.dbPath != "/tmp/test.db" {
		t.Errorf("dbPath = %q, want /tmp/test.db", got.dbPath)
	}
	if want := (reminder.TimeOfDay{Hour: 8, Minute: 30}); got.reminderTime != want {
		t.Errorf("reminderTime = %+v, want %+v", got.reminderTime, want)
	}
}

func TestLoadConfigRejectsInvalidReminderTime(t *testing.T) {
	if _, err := loadConfig(env(map[string]string{"REMINDER_TIME": "9:00"})); err == nil {
		t.Error("loadConfig with REMINDER_TIME=9:00 gives no error")
	}
}
