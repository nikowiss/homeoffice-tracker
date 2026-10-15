package civil

import "time"

const layout = "2006-01-02"

type Date struct {
	Year  int
	Month time.Month
	Day   int
}

func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{Year: y, Month: m, Day: d}
}

func ParseDate(s string) (Date, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return Date{}, err
	}
	return DateOf(t), nil
}

func (d Date) String() string {
	return d.midnight().Format(layout)
}

func (d Date) Format(layout string) string {
	return d.midnight().Format(layout)
}

func (d Date) After(other Date) bool {
	return d.midnight().After(other.midnight())
}

func (d Date) Compare(other Date) int {
	return d.midnight().Compare(other.midnight())
}

func (d Date) AddDays(n int) Date {
	return DateOf(d.midnight().AddDate(0, 0, n))
}

func (d Date) Weekday() time.Weekday {
	return d.midnight().Weekday()
}

func (d Date) midnight() time.Time {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC)
}
