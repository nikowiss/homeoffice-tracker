package store

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"

	"homeoffice-tracker/internal/civil"
)

const (
	markSQL   = `INSERT OR IGNORE INTO home_office_days (date) VALUES (?)`
	unmarkSQL = `DELETE FROM home_office_days WHERE date = ?`
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS home_office_days (date TEXT PRIMARY KEY)`); err != nil {
		db.Close()
		return nil, err
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
