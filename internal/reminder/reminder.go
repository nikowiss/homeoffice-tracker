package reminder

import (
	"fmt"
	"time"

	"homeoffice-tracker/internal/civil"
)

type TimeOfDay struct {
	Hour   int
	Minute int
}

func ParseTimeOfDay(raw string) (TimeOfDay, error) {
	if raw == "" {
		return TimeOfDay{Hour: 9}, nil
	}
	t, err := time.Parse("15:04", raw)
	if err != nil || len(raw) != len("15:04") {
		return TimeOfDay{}, fmt.Errorf("invalid REMINDER_TIME %q: want HH:MM", raw)
	}
	return TimeOfDay{Hour: t.Hour(), Minute: t.Minute()}, nil
}

func Due(now time.Time, at TimeOfDay, todayMarked bool, lastReminder civil.Date) bool {
	local := now.In(civil.Berlin)
	today := civil.DateOf(local)
	switch {
	case today.Weekday() == time.Saturday || today.Weekday() == time.Sunday:
		return false
	case todayMarked || lastReminder == today:
		return false
	}
	reminderTime := time.Date(local.Year(), local.Month(), local.Day(), at.Hour, at.Minute, 0, 0, civil.Berlin)
	return !local.Before(reminderTime)
}

func SecondsUntilMidnight(now time.Time) int {
	local := now.In(civil.Berlin)
	midnight := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, civil.Berlin)
	return int(midnight.Sub(local).Seconds())
}
