package store

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

const dateLayout = "2006-01-02"

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

func (s *Store) Mark(day time.Time) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO home_office_days (date) VALUES (?)`, day.Format(dateLayout))
	return err
}

func (s *Store) Unmark(day time.Time) error {
	_, err := s.db.Exec(`DELETE FROM home_office_days WHERE date = ?`, day.Format(dateLayout))
	return err
}

func (s *Store) Days(year int) ([]time.Time, error) {
	rows, err := s.db.Query(
		`SELECT date FROM home_office_days WHERE date >= ? AND date < ? ORDER BY date`,
		fmt.Sprintf("%04d-01-01", year), fmt.Sprintf("%04d-01-01", year+1),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var days []time.Time
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		day, err := time.Parse(dateLayout, raw)
		if err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, rows.Err()
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
