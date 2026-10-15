package store

import (
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"

	"homeoffice-tracker/internal/civil"
)

const (
	markSQL   = `INSERT OR IGNORE INTO home_office_days (date) VALUES (?)`
	unmarkSQL = `DELETE FROM home_office_days WHERE date = ?`
)

var schema = []string{
	`CREATE TABLE IF NOT EXISTS home_office_days (date TEXT PRIMARY KEY)`,
	`CREATE TABLE IF NOT EXISTS push_subscriptions (endpoint TEXT PRIMARY KEY, p256dh TEXT NOT NULL, auth TEXT NOT NULL)`,
	`CREATE TABLE IF NOT EXISTS settings (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range schema {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Mark(day civil.Date) error {
	_, err := s.db.Exec(markSQL, day.String())
	return err
}

func (s *Store) Marked(day civil.Date) (bool, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM home_office_days WHERE date = ?`, day.String()).Scan(&count)
	return count > 0, err
}

func (s *Store) Unmark(day civil.Date) error {
	_, err := s.db.Exec(unmarkSQL, day.String())
	return err
}

func (s *Store) Apply(mark, unmark []civil.Date) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, day := range mark {
		if _, err := tx.Exec(markSQL, day.String()); err != nil {
			return err
		}
	}
	for _, day := range unmark {
		if _, err := tx.Exec(unmarkSQL, day.String()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Days(year int) ([]civil.Date, error) {
	rows, err := s.db.Query(
		`SELECT date FROM home_office_days WHERE date >= ? AND date < ? ORDER BY date`,
		newYear(year).String(), newYear(year+1).String(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []civil.Date
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		day, err := civil.ParseDate(raw)
		if err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
}

func newYear(year int) civil.Date {
	return civil.Date{Year: year, Month: time.January, Day: 1}
}

func (s *Store) YearCounts() (map[int]int, error) {
	rows, err := s.db.Query(`SELECT CAST(substr(date, 1, 4) AS INTEGER), COUNT(*) FROM home_office_days GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[int]int{}
	for rows.Next() {
		var year, count int
		if err := rows.Scan(&year, &count); err != nil {
			return nil, err
		}
		counts[year] = count
	}
	return counts, rows.Err()
}

type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

func (s *Store) SaveSubscription(sub Subscription) error {
	_, err := s.db.Exec(
		`INSERT INTO push_subscriptions (endpoint, p256dh, auth) VALUES (?, ?, ?)
		 ON CONFLICT (endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth`,
		sub.Endpoint, sub.P256dh, sub.Auth,
	)
	return err
}

func (s *Store) DeleteSubscription(endpoint string) error {
	_, err := s.db.Exec(`DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

func (s *Store) Subscriptions() ([]Subscription, error) {
	rows, err := s.db.Query(`SELECT endpoint, p256dh, auth FROM push_subscriptions ORDER BY endpoint`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var subs []Subscription
	for rows.Next() {
		var sub Subscription
		if err := rows.Scan(&sub.Endpoint, &sub.P256dh, &sub.Auth); err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

const (
	vapidPublicKey   = "vapid_public_key"
	vapidPrivateKey  = "vapid_private_key"
	lastReminderDate = "last_reminder_date"
)

type VAPIDKeys struct {
	Private string
	Public  string
}

func (s *Store) VAPIDKeys(generate func() (private, public string, err error)) (VAPIDKeys, error) {
	var keys VAPIDKeys
	err := s.db.QueryRow(
		`SELECT private.value, public.value FROM settings private, settings public WHERE private.key = ? AND public.key = ?`,
		vapidPrivateKey, vapidPublicKey,
	).Scan(&keys.Private, &keys.Public)
	if err == nil {
		return keys, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return VAPIDKeys{}, err
	}

	keys.Private, keys.Public, err = generate()
	if err != nil {
		return VAPIDKeys{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return VAPIDKeys{}, err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{vapidPrivateKey: keys.Private, vapidPublicKey: keys.Public} {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, key, value); err != nil {
			return VAPIDKeys{}, err
		}
	}
	return keys, tx.Commit()
}

func (s *Store) LastReminderDate() (civil.Date, error) {
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, lastReminderDate).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return civil.Date{}, nil
	}
	if err != nil {
		return civil.Date{}, err
	}
	return civil.ParseDate(raw)
}

func (s *Store) SetLastReminderDate(day civil.Date) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`, lastReminderDate, day.String())
	return err
}
