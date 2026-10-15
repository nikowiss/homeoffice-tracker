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

//go:embed static
var staticFS embed.FS

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

type ReminderSetup struct {
	PublicKey string
	SendNow   func() error
}

type server struct {
	store     *store.Store
	now       func() time.Time
	reminders ReminderSetup
}

func NewHandler(s *store.Store, now func() time.Time, reminders ReminderSetup) http.Handler {
	srv := &server{store: s, now: now, reminders: reminders}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", srv.index)
	mux.HandleFunc("POST /today", srv.markToday)
	mux.HandleFunc("POST /today/delete", srv.unmarkToday)
	mux.HandleFunc("POST /selection", srv.confirmSelection)
	mux.HandleFunc("POST /days/{date}", srv.markTodayCell)
	mux.HandleFunc("POST /days/{date}/delete", srv.unmarkTodayCell)
	mux.HandleFunc("GET /export/{year}", srv.export)
	mux.HandleFunc("POST /reminder/subscriptions", srv.subscribe)
	mux.HandleFunc("POST /reminder/subscriptions/delete", srv.unsubscribe)
	mux.HandleFunc("POST /reminder/test", srv.sendTestReminder)
	mux.HandleFunc("GET /style.css", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, templateFS, "templates/style.css")
	})
	for _, name := range []string{"favicon.ico", "favicon.svg", "apple-touch-icon.png", "sw.js", "reminder.js"} {
		mux.HandleFunc("GET /"+name, func(w http.ResponseWriter, r *http.Request) {
			http.ServeFileFS(w, r, staticFS, "static/"+name)
		})
	}
	return mux
}

func (s *server) today() civil.Date {
	return civil.DateOf(s.now().In(berlin))
}

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

func (s *server) markToday(w http.ResponseWriter, r *http.Request) {
	s.changeToday(w, r, s.store.Mark)
}

func (s *server) unmarkToday(w http.ResponseWriter, r *http.Request) {
	s.changeToday(w, r, s.store.Unmark)
}

func (s *server) changeToday(w http.ResponseWriter, r *http.Request, write func(civil.Date) error) {
	today := s.today()
	state, err := parsePageForm(r, today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := write(today); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, state.url(), http.StatusSeeOther)
}

func pathDate(w http.ResponseWriter, r *http.Request) (civil.Date, bool) {
	day, err := civil.ParseDate(r.PathValue("date"))
	if err != nil {
		http.Error(w, "invalid date", http.StatusBadRequest)
		return civil.Date{}, false
	}
	return day, true
}

func (s *server) markTodayCell(w http.ResponseWriter, r *http.Request) {
	s.changeTodayCell(w, r, s.store.Mark)
}

func (s *server) unmarkTodayCell(w http.ResponseWriter, r *http.Request) {
	s.changeTodayCell(w, r, s.store.Unmark)
}

func (s *server) changeTodayCell(w http.ResponseWriter, r *http.Request, write func(civil.Date) error) {
	day, ok := pathDate(w, r)
	if !ok {
		return
	}
	today := s.today()
	if day.After(today) {
		http.Error(w, "date is after today", http.StatusBadRequest)
		return
	}
	state, err := parsePageForm(r, today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if state.month == (civil.Date{}) {
		state.month = firstOfMonth(day.Year, day.Month)
	}
	if day == today {
		if err := write(day); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		state.selection = state.selection.with(day)
	}
	http.Redirect(w, r, state.url(), http.StatusSeeOther)
}

type pageState struct {
	month     civil.Date
	selection selection
}

func parsePageForm(r *http.Request, today civil.Date) (pageState, error) {
	if err := r.ParseForm(); err != nil {
		return pageState{}, err
	}
	state := pageState{selection: parseSelection(r.PostForm["select"], today)}
	raw := r.PostForm.Get("month")
	if raw == "" {
		return state, nil
	}
	month, err := parseMonth(raw)
	if err != nil {
		return pageState{}, fmt.Errorf("invalid month %q", raw)
	}
	if !month.After(today) {
		state.month = month
	}
	return state, nil
}

func (p pageState) url() string {
	query := url.Values{}
	if p.month != (civil.Date{}) {
		query.Set("month", monthParam(p.month))
	}
	for _, day := range p.selection {
		query.Add("select", day.String())
	}
	if len(query) == 0 {
		return "/"
	}
	return "/?" + query.Encode()
}

type selection []civil.Date

func parseSelection(raw []string, today civil.Date) selection {
	var days selection
	for _, value := range raw {
		day, err := parsePastDay(value, today)
		if err != nil {
			continue
		}
		days = days.with(day)
	}
	return days
}

func (s selection) contains(day civil.Date) bool {
	return slices.Contains(s, day)
}

func (s selection) with(day civil.Date) selection {
	if s.contains(day) {
		return s
	}
	days := append(slices.Clone(s), day)
	slices.SortFunc(days, civil.Date.Compare)
	return days
}

func (s selection) toggled(day civil.Date) selection {
	if s.contains(day) {
		return slices.DeleteFunc(slices.Clone(s), func(selected civil.Date) bool { return selected == day })
	}
	return s.with(day)
}

func parseMonth(raw string) (civil.Date, error) {
	parsed, err := time.Parse(monthLayout, raw)
	if err != nil {
		return civil.Date{}, err
	}
	return civil.DateOf(parsed), nil
}

func parsePastDay(raw string, today civil.Date) (civil.Date, error) {
	day, err := civil.ParseDate(raw)
	if err != nil {
		return civil.Date{}, err
	}
	if !today.After(day) {
		return civil.Date{}, fmt.Errorf("%s is not a past day", raw)
	}
	return day, nil
}

func parsePastDays(raw []string, today civil.Date) ([]civil.Date, error) {
	days := make([]civil.Date, 0, len(raw))
	for _, value := range raw {
		day, err := parsePastDay(value, today)
		if err != nil {
			return nil, err
		}
		days = append(days, day)
	}
	return days, nil
}

func (s *server) confirmSelection(w http.ResponseWriter, r *http.Request) {
	today := s.today()
	state, err := parsePageForm(r, today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
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
	http.Redirect(w, r, pageState{month: state.month}.url(), http.StatusSeeOther)
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
