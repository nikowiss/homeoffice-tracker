package web

import (
	"embed"
	"encoding/csv"
	"fmt"
	"html/template"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
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
	mux.HandleFunc("POST /selection", srv.applySelection)
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
	Month       string
	Selection   []civil.Date
	MonthTitle  string
	PrevHref    string
	NextHref    string
	Weeks       [][]calendarDay
	YearCounts  []yearCount
	Bar         *confirmBar
}

type confirmBar struct {
	Summary    string
	Dates      []barDate
	Mark       []civil.Date
	Unmark     []civil.Date
	CancelHref string
}

type barDate struct {
	Label  string
	Unmark bool
}

type calendarDay struct {
	Day      int
	Date     string
	Href     string
	Marked   bool
	Selected bool
	Future   bool
	IsToday  bool
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
		parsed, err := parseMonth(raw)
		if err != nil {
			http.Error(w, "invalid month", http.StatusBadRequest)
			return
		}
		if parsed.After(month) {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		month = parsed
	}
	selection := parseSelection(r.URL.Query()["select"], today)
	years := []int{today.Year, month.Year}
	for _, day := range selection {
		years = append(years, day.Year)
	}
	marked, err := s.markedLookup(years)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	counts, err := s.store.YearCounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	shownMonth := monthParam(month)
	data := pageData{
		TodayMarked: marked(today),
		Month:       shownMonth,
		Selection:   selection,
		MonthTitle:  fmt.Sprintf("%s %d", month.Month, month.Year),
		PrevHref:    pageURL(monthParam(firstOfMonth(month.Year, month.Month-1)), selection),
		Weeks:       calendarWeeks(month, today, selection, marked),
		YearCounts:  yearCountsWithCurrent(counts, today.Year),
	}
	if next := firstOfMonth(month.Year, month.Month+1); !next.After(today) {
		data.NextHref = pageURL(monthParam(next), selection)
	}
	if len(selection) > 0 {
		data.Bar = newConfirmBar(selection, marked, shownMonth)
	}
	if err := pageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *server) markedLookup(years []int) (func(civil.Date) bool, error) {
	byYear := map[int][]civil.Date{}
	for _, year := range years {
		if _, ok := byYear[year]; ok {
			continue
		}
		days, err := s.store.Days(year)
		if err != nil {
			return nil, err
		}
		byYear[year] = days
	}
	return func(day civil.Date) bool {
		return slices.Contains(byYear[day.Year], day)
	}, nil
}

func calendarWeeks(month, today civil.Date, selection []civil.Date, marked func(civil.Date) bool) [][]calendarDay {
	shownMonth := monthParam(month)
	leadingBlanks := (int(month.Weekday()) + 6) % 7
	cells := make([]calendarDay, leadingBlanks)
	for day := month; day.Month == month.Month; day = day.AddDays(1) {
		cells = append(cells, calendarDay{
			Day:      day.Day,
			Date:     day.String(),
			Href:     pageURL(shownMonth, toggled(selection, day)),
			Marked:   marked(day),
			Selected: slices.Contains(selection, day),
			Future:   day.After(today),
			IsToday:  day == today,
		})
	}
	for len(cells)%7 != 0 {
		cells = append(cells, calendarDay{})
	}
	return slices.Collect(slices.Chunk(cells, 7))
}

func newConfirmBar(selection []civil.Date, marked func(civil.Date) bool, shownMonth string) *confirmBar {
	bar := &confirmBar{CancelHref: pageURL(shownMonth, nil)}
	for _, day := range selection {
		if marked(day) {
			bar.Unmark = append(bar.Unmark, day)
		} else {
			bar.Mark = append(bar.Mark, day)
		}
	}
	var parts []string
	if len(bar.Mark) > 0 {
		parts = append(parts, "mark "+dayCount(len(bar.Mark)))
	}
	if len(bar.Unmark) > 0 {
		parts = append(parts, "unmark "+dayCount(len(bar.Unmark)))
	}
	summary := strings.Join(parts, ", ")
	bar.Summary = strings.ToUpper(summary[:1]) + summary[1:]
	for _, day := range bar.Mark {
		bar.Dates = append(bar.Dates, newBarDate(day, false))
	}
	for _, day := range bar.Unmark {
		bar.Dates = append(bar.Dates, newBarDate(day, true))
	}
	return bar
}

func newBarDate(day civil.Date, unmark bool) barDate {
	return barDate{Label: day.Format("Mon 2 Jan 2006"), Unmark: unmark}
}

func dayCount(n int) string {
	if n == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", n)
}

func toggled(selection []civil.Date, day civil.Date) []civil.Date {
	if slices.Contains(selection, day) {
		return slices.DeleteFunc(slices.Clone(selection), func(selected civil.Date) bool { return selected == day })
	}
	return sortedDays(append(slices.Clone(selection), day))
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
	s.changeToday(w, r, s.store.Mark)
}

func (s *server) unmarkToday(w http.ResponseWriter, r *http.Request) {
	s.changeToday(w, r, s.store.Unmark)
}

func (s *server) changeToday(w http.ResponseWriter, r *http.Request, write func(civil.Date) error) {
	today := s.today()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	month, err := formMonth(r, today)
	if err != nil {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}
	if err := write(today); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, pageURL(month, parseSelection(r.PostForm["select"], today)), http.StatusSeeOther)
}

func pathDate(w http.ResponseWriter, r *http.Request) (civil.Date, bool) {
	day, err := civil.ParseDate(r.PathValue("date"))
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return civil.Date{}, false
	}
	return day, true
}

func (s *server) markDay(w http.ResponseWriter, r *http.Request) {
	s.changeDay(w, r, s.store.Mark)
}

func (s *server) unmarkDay(w http.ResponseWriter, r *http.Request) {
	s.changeDay(w, r, s.store.Unmark)
}

func (s *server) changeDay(w http.ResponseWriter, r *http.Request, write func(civil.Date) error) {
	day, ok := pathDate(w, r)
	if !ok {
		return
	}
	today := s.today()
	if day.After(today) {
		http.Error(w, "date is after today", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	month, err := formMonth(r, today)
	if err != nil {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}
	if month == "" {
		month = monthParam(day)
	}
	selection := parseSelection(r.PostForm["select"], today)
	if day == today {
		if err := write(day); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else if !slices.Contains(selection, day) {
		selection = sortedDays(append(selection, day))
	}
	http.Redirect(w, r, pageURL(month, selection), http.StatusSeeOther)
}

func parseSelection(raw []string, today civil.Date) []civil.Date {
	var selection []civil.Date
	for _, value := range raw {
		day, err := civil.ParseDate(value)
		if err != nil || !today.After(day) || slices.Contains(selection, day) {
			continue
		}
		selection = append(selection, day)
	}
	return sortedDays(selection)
}

func sortedDays(days []civil.Date) []civil.Date {
	slices.SortFunc(days, civil.Date.Compare)
	return days
}

func pageURL(month string, selection []civil.Date) string {
	query := url.Values{}
	if month != "" {
		query.Set("month", month)
	}
	for _, day := range selection {
		query.Add("select", day.String())
	}
	if len(query) == 0 {
		return "/"
	}
	return "/?" + query.Encode()
}

func parseMonth(raw string) (civil.Date, error) {
	parsed, err := time.Parse(monthLayout, raw)
	if err != nil {
		return civil.Date{}, err
	}
	return civil.DateOf(parsed), nil
}

func formMonth(r *http.Request, today civil.Date) (string, error) {
	raw := r.PostForm.Get("month")
	if raw == "" {
		return "", nil
	}
	month, err := parseMonth(raw)
	if err != nil {
		return "", err
	}
	if month.After(today) {
		return "", fmt.Errorf("month %s is after today", raw)
	}
	return raw, nil
}

func parsePastDays(raw []string, today civil.Date) ([]civil.Date, error) {
	days := make([]civil.Date, 0, len(raw))
	for _, value := range raw {
		day, err := civil.ParseDate(value)
		if err != nil {
			return nil, err
		}
		if !today.After(day) {
			return nil, fmt.Errorf("%s is not a past day", value)
		}
		days = append(days, day)
	}
	return days, nil
}

func (s *server) applySelection(w http.ResponseWriter, r *http.Request) {
	today := s.today()
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	month, err := formMonth(r, today)
	if err != nil {
		http.Error(w, "invalid month", http.StatusBadRequest)
		return
	}
	mark, err := parsePastDays(r.PostForm["mark"], today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	unmark, err := parsePastDays(r.PostForm["unmark"], today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, day := range mark {
		if slices.Contains(unmark, day) {
			http.Error(w, fmt.Sprintf("%s is in mark and unmark", day), http.StatusBadRequest)
			return
		}
	}
	if len(mark) > 0 || len(unmark) > 0 {
		if err := s.store.Apply(mark, unmark); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	http.Redirect(w, r, pageURL(month, nil), http.StatusSeeOther)
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
