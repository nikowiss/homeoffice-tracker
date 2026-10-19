package web

import (
	"bytes"
	"context"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"homeoffice-tracker/internal/civil"
)

type pageData struct {
	TodayMarked     bool
	Month           string
	Selection       selection
	MonthTitle      string
	PrevHref        string
	NextHref        string
	Weeks           [][]calendarDay
	YearCounts      []yearCount
	ConfirmationBar *confirmationBar
	VAPIDPublicKey  string
}

type confirmationBar struct {
	Summary    string
	Days       []selectedDay
	CancelHref string
}

type selectedDay struct {
	Date   civil.Date
	Unmark bool
}

func newSelectedDay(day civil.Date, marked func(civil.Date) bool) selectedDay {
	return selectedDay{Date: day, Unmark: marked(day)}
}

func (d selectedDay) Label() string {
	return d.Date.Format("Mon 2 Jan 2006")
}

type calendarDay struct {
	Day     int
	Date    string
	Href    string
	Class   string
	Marked  bool
	Future  bool
	IsToday bool
}

type yearCount struct {
	Year    int
	Count   int
	Current bool
}

func firstOfMonth(year int, month time.Month) civil.Date {
	return civil.DateOf(time.Date(year, month, 1, 0, 0, 0, 0, time.UTC))
}

func monthParam(day civil.Date) string {
	return fmt.Sprintf("%04d-%02d", day.Year, day.Month)
}

func (s *server) index(w http.ResponseWriter, r *http.Request) {
	today := s.today()
	state := pageState{
		month:     firstOfMonth(today.Year, today.Month),
		selection: parseSelection(r.URL.Query()["select"], today),
	}
	if raw := r.URL.Query().Get("month"); raw != "" {
		parsed, err := parseMonth(raw)
		if err != nil {
			http.Error(w, "invalid month", http.StatusBadRequest)
			return
		}
		if parsed.After(state.month) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		state.month = parsed
	}
	month := state.month
	years := []int{today.Year, month.Year}
	for _, day := range state.selection {
		years = append(years, day.Year)
	}
	marked, err := s.markedLookup(r.Context(), years)
	if err != nil {
		serverError(w, r, err)
		return
	}
	counts, err := s.store.YearCounts(r.Context())
	if err != nil {
		serverError(w, r, err)
		return
	}
	data := pageData{
		TodayMarked:    marked(today),
		Month:          monthParam(month),
		Selection:      state.selection,
		MonthTitle:     fmt.Sprintf("%s %d", month.Month, month.Year),
		PrevHref:       pageState{month: firstOfMonth(month.Year, month.Month-1), selection: state.selection}.url(),
		Weeks:          calendarWeeks(state, today, marked),
		YearCounts:     yearCountsWithCurrent(counts, today.Year),
		VAPIDPublicKey: s.reminders.PublicKey,
	}
	if next := firstOfMonth(month.Year, month.Month+1); !next.After(today) {
		data.NextHref = pageState{month: next, selection: state.selection}.url()
	}
	if len(state.selection) > 0 {
		data.ConfirmationBar = newConfirmationBar(state, marked)
	}
	var page bytes.Buffer
	if err := pageTemplate.Execute(&page, data); err != nil {
		serverError(w, r, err)
		return
	}
	page.WriteTo(w)
}

func (s *server) markedLookup(ctx context.Context, years []int) (func(civil.Date) bool, error) {
	byYear := map[int][]civil.Date{}
	for _, year := range years {
		if _, ok := byYear[year]; ok {
			continue
		}
		days, err := s.store.Days(ctx, year)
		if err != nil {
			return nil, err
		}
		byYear[year] = days
	}
	return func(day civil.Date) bool {
		return slices.Contains(byYear[day.Year], day)
	}, nil
}

func calendarWeeks(state pageState, today civil.Date, marked func(civil.Date) bool) [][]calendarDay {
	month := state.month
	leadingBlanks := (int(month.Weekday()) + 6) % 7
	cells := make([]calendarDay, leadingBlanks)
	for day := month; day.Month == month.Month; day = day.AddDays(1) {
		cells = append(cells, calendarDay{
			Day:     day.Day,
			Date:    day.String(),
			Href:    pageState{month: month, selection: state.selection.toggled(day)}.url(),
			Class:   cellClass(day, state.selection, marked),
			Marked:  marked(day),
			Future:  day.After(today),
			IsToday: day == today,
		})
	}
	for len(cells)%7 != 0 {
		cells = append(cells, calendarDay{})
	}
	return slices.Collect(slices.Chunk(cells, 7))
}

func cellClass(day civil.Date, selection selection, marked func(civil.Date) bool) string {
	if selection.contains(day) {
		if newSelectedDay(day, marked).Unmark {
			return "select-unmark"
		}
		return "select-mark"
	}
	if marked(day) {
		return "marked"
	}
	return ""
}

func newConfirmationBar(state pageState, marked func(civil.Date) bool) *confirmationBar {
	bar := &confirmationBar{CancelHref: pageState{month: state.month}.url()}
	var mark, unmark int
	for _, day := range state.selection {
		selected := newSelectedDay(day, marked)
		bar.Days = append(bar.Days, selected)
		if selected.Unmark {
			unmark++
		} else {
			mark++
		}
	}
	bar.Summary = summary(mark, unmark)
	return bar
}

func summary(mark, unmark int) string {
	var parts []string
	if mark > 0 {
		parts = append(parts, "Mark "+dayCount(mark))
	}
	if unmark > 0 {
		verb := "Unmark"
		if mark > 0 {
			verb = "unmark"
		}
		parts = append(parts, verb+" "+dayCount(unmark))
	}
	return strings.Join(parts, ", ")
}

func dayCount(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

func yearCountsWithCurrent(counts map[int]int, currentYear int) []yearCount {
	if _, ok := counts[currentYear]; !ok {
		counts[currentYear] = 0
	}
	years := slices.Sorted(maps.Keys(counts))
	slices.Reverse(years)

	rows := make([]yearCount, 0, len(years))
	for _, year := range years {
		rows = append(rows, yearCount{Year: year, Count: counts[year], Current: year == currentYear})
	}
	return rows
}
