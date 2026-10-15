package web

import (
	"embed"
	"encoding/csv"
	"fmt"
	"html/template"
	"maps"
	"net/http"
	"slices"
	"strconv"
	"time"
	_ "time/tzdata"

	"homeoffice-tracker/internal/civil"
	"homeoffice-tracker/internal/store"
)

//go:embed templates/*.html templates/style.css
var templateFS embed.FS

const monthLayout = "2006-01"

var berlin = mustLoadLocation("Europe/Berlin")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

var pageTemplate = template.Must(template.ParseFS(templateFS, "templates/index.html"))

type server struct {
	store *store.Store
	now   func() time.Time
}

func NewHandler(s *store.Store, now func() time.Time) http.Handler {
	srv := &server{store: s, now: now}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", srv.index)
	mux.HandleFunc("POST /today", srv.markToday)
	mux.HandleFunc("POST /today/delete", srv.unmarkToday)
	mux.HandleFunc("POST /days/{date}", srv.markDay)
	mux.HandleFunc("POST /days/{date}/delete", srv.unmarkDay)
	mux.HandleFunc("GET /export/{year}", srv.export)
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, templateFS, "templates/style.css")
	})
	return mux
}

func (s *server) today() civil.Date {
	return civil.DateOf(s.now().In(berlin))
}

type pageData struct {
	TodayMarked bool
	MonthTitle  string
	PrevMonth   string
	NextMonth   string
	Weeks       [][]calendarDay
	YearCounts  []yearCount
}

type calendarDay struct {
	Day     int
	Date    string
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
	month := firstOfMonth(today.Year, today.Month)
	if raw := r.URL.Query().Get("month"); raw != "" {
		parsed, err := time.Parse(monthLayout, raw)
		if err != nil {
			http.Error(w, "invalid month", http.StatusBadRequest)
			return
		}
		if civil.DateOf(parsed).After(month) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		month = civil.DateOf(parsed)
	}
	currentYearDays, err := s.store.Days(today.Year)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	selectedYearDays, err := s.store.Days(month.Year)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	counts, err := s.store.YearCounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := pageData{
		TodayMarked: slices.Contains(currentYearDays, today),
		MonthTitle:  fmt.Sprintf("%s %d", month.Month, month.Year),
		PrevMonth:   monthParam(firstOfMonth(month.Year, month.Month-1)),
		Weeks:       calendarWeeks(month, today, selectedYearDays),
		YearCounts:  yearCountsWithCurrent(counts, today.Year),
	}
	if next := firstOfMonth(month.Year, month.Month+1); !next.After(today) {
		data.NextMonth = monthParam(next)
	}
	if err := pageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func calendarWeeks(month, today civil.Date, marked []civil.Date) [][]calendarDay {
	leadingBlanks := (int(month.Weekday()) + 6) % 7
	cells := make([]calendarDay, leadingBlanks)
	for day := month; day.Month == month.Month; day = day.AddDays(1) {
		cells = append(cells, calendarDay{
			Day:     day.Day,
			Date:    day.String(),
			Marked:  slices.Contains(marked, day),
			Future:  day.After(today),
			IsToday: day == today,
		})
	}
	for len(cells)%7 != 0 {
		cells = append(cells, calendarDay{})
	}
	return slices.Collect(slices.Chunk(cells, 7))
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

func (s *server) markToday(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Mark(s.today()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) unmarkToday(w http.ResponseWriter, r *http.Request) {
	if err := s.store.Unmark(s.today()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func pathDate(w http.ResponseWriter, r *http.Request) (civil.Date, bool) {
	day, err := civil.ParseDate(r.PathValue("date"))
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return civil.Date{}, false
	}
	return day, true
}

func redirectToMonthOf(w http.ResponseWriter, r *http.Request, day civil.Date) {
	http.Redirect(w, r, "/?month="+monthParam(day), http.StatusSeeOther)
}

func (s *server) markDay(w http.ResponseWriter, r *http.Request) {
	day, ok := pathDate(w, r)
	if !ok {
		return
	}
	if day.After(s.today()) {
		http.Error(w, "date is after today", http.StatusBadRequest)
		return
	}
	if err := s.store.Mark(day); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirectToMonthOf(w, r, day)
}

func (s *server) unmarkDay(w http.ResponseWriter, r *http.Request) {
	day, ok := pathDate(w, r)
	if !ok {
		return
	}
	if err := s.store.Unmark(day); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	redirectToMonthOf(w, r, day)
}

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	days, err := s.store.Days(year)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="homeoffice-%d.csv"`, year))
	out := csv.NewWriter(w)
	out.Write([]string{"date", "weekday"})
	for _, day := range days {
		out.Write([]string{day.String(), day.Weekday().String()})
	}
	out.Flush()
}
