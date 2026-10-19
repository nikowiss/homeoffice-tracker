package store

import (
	"context"
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

func Open(ctx context.Context, path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, statement := range schema {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Mark(ctx context.Context, day civil.Date) error {
	_, err := s.db.ExecContext(ctx, markSQL, day.String())
	return err
}

func (s *Store) Marked(ctx context.Context, day civil.Date) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM home_office_days WHERE date = ?`, day.String()).Scan(&count)
	return count > 0, err
}

func (s *Store) Unmark(ctx context.Context, day civil.Date) error {
	_, err := s.db.ExecContext(ctx, unmarkSQL, day.String())
	return err
}

func (s *Store) Apply(ctx context.Context, mark, unmark []civil.Date) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, day := range mark {
		if _, err := tx.ExecContext(ctx, markSQL, day.String()); err != nil {
			return err
		}
	}
	for _, day := range unmark {
		if _, err := tx.ExecContext(ctx, unmarkSQL, day.String()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Days(ctx context.Context, year int) ([]civil.Date, error) {
	rows, err := s.db.QueryContext(ctx,
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

func (s *Store) YearCounts(ctx context.Context) (map[int]int, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT CAST(substr(date, 1, 4) AS INTEGER), COUNT(*) FROM home_office_days GROUP BY 1`)
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

func (s *Store) SaveSubscription(ctx context.Context, sub Subscription) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO push_subscriptions (endpoint, p256dh, auth) VALUES (?, ?, ?)
		 ON CONFLICT (endpoint) DO UPDATE SET p256dh = excluded.p256dh, auth = excluded.auth`,
		sub.Endpoint, sub.P256dh, sub.Auth,
	)
	return err
}

func (s *Store) DeleteSubscription(ctx context.Context, endpoint string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM push_subscriptions WHERE endpoint = ?`, endpoint)
	return err
}

func (s *Store) Subscriptions(ctx context.Context) ([]Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT endpoint, p256dh, auth FROM push_subscriptions ORDER BY endpoint`)
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

const setSettingSQL = `INSERT OR REPLACE INTO settings (key, value) VALUES (?, ?)`

const (
	vapidPublicKey   = "vapid_public_key"
	vapidPrivateKey  = "vapid_private_key"
	lastReminderDate = "last_reminder_date"
)

type VAPIDKeys struct {
	Private string
	Public  string
}

func (s *Store) VAPIDKeys(ctx context.Context, generate func() (private, public string, err error)) (VAPIDKeys, error) {
	var keys VAPIDKeys
	err := s.db.QueryRowContext(ctx,
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VAPIDKeys{}, err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{vapidPrivateKey: keys.Private, vapidPublicKey: keys.Public} {
		if _, err := tx.ExecContext(ctx, setSettingSQL, key, value); err != nil {
			return VAPIDKeys{}, err
		}
	}
	return keys, tx.Commit()
}

func (s *Store) LastReminderDate(ctx context.Context) (civil.Date, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, lastReminderDate).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return civil.Date{}, nil
	}
	if err != nil {
		return civil.Date{}, err
	}
	return civil.ParseDate(raw)
}

func (s *Store) SetLastReminderDate(ctx context.Context, day civil.Date) error {
	_, err := s.db.ExecContext(ctx, setSettingSQL, lastReminderDate, day.String())
	return err
}
