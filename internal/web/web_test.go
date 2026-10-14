package web_test

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"homeoffice-tracker/internal/store"
	"homeoffice-tracker/internal/web"
)

func newHandler(t *testing.T, now time.Time) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "homeoffice.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return web.NewHandler(s, func() time.Time { return now })
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func assertRedirect(t *testing.T, rec *httptest.ResponseRecorder, location string) {
	t.Helper()
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusSeeOther, rec.Body)
	}
	if got := rec.Header().Get("Location"); got != location {
		t.Errorf("Location = %q, want %q", got, location)
	}
}

func assertStatus(t *testing.T, rec *httptest.ResponseRecorder, status int) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body: %s", rec.Code, status, rec.Body)
	}
}

func assertContains(t *testing.T, body, want string) {
	t.Helper()
	if !strings.Contains(body, want) {
		t.Errorf("body does not contain %q:\n%s", want, body)
	}
}

func assertNotContains(t *testing.T, body, unwanted string) {
	t.Helper()
	if strings.Contains(body, unwanted) {
		t.Errorf("body contains %q:\n%s", unwanted, body)
	}
}

var berlin = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		panic(err)
	}
	return loc
}()

var wednesdayMorning = time.Date(2026, 10, 14, 10, 0, 0, 0, berlin)

func TestOwnerMarksTodayWithOneClick(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	before := do(t, h, http.MethodGet, "/")
	assertStatus(t, before, http.StatusOK)
	assertContains(t, before.Body.String(), "Mark today as home office day")

	assertRedirect(t, do(t, h, http.MethodPost, "/today"), "/")

	after := do(t, h, http.MethodGet, "/")
	assertContains(t, after.Body.String(), "Today is a home office day")
	assertNotContains(t, after.Body.String(), "Mark today as home office day")
}

func TestExportListsTheYearAsCSV(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	do(t, h, http.MethodPost, "/today")

	rec := do(t, h, http.MethodGet, "/export/2026")

	assertStatus(t, rec, http.StatusOK)
	if got := rec.Header().Get("Content-Type"); got != "text/csv; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Disposition"); got != `attachment; filename="homeoffice-2026.csv"` {
		t.Errorf("Content-Disposition = %q", got)
	}
	if got, want := rec.Body.String(), "date,weekday\n2026-10-14,Wednesday\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestTodayIsTheDateInBerlin(t *testing.T) {
	halfPastMidnightInBerlin := time.Date(2026, 12, 31, 23, 30, 0, 0, time.UTC)
	h := newHandler(t, halfPastMidnightInBerlin)

	do(t, h, http.MethodPost, "/today")

	got := do(t, h, http.MethodGet, "/export/2027").Body.String()
	if want := "date,weekday\n2027-01-01,Friday\n"; got != want {
		t.Errorf("export 2027 = %q, want %q", got, want)
	}
}

func TestOwnerMarksPastDaysIncludingWeekends(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	assertRedirect(t, do(t, h, http.MethodPost, "/days/2026-03-08"), "/?month=2026-03")
	assertRedirect(t, do(t, h, http.MethodPost, "/days/2026-01-05"), "/?month=2026-01")

	got := do(t, h, http.MethodGet, "/export/2026").Body.String()
	if want := "date,weekday\n2026-01-05,Monday\n2026-03-08,Sunday\n"; got != want {
		t.Errorf("export 2026 = %q, want %q", got, want)
	}
}

func TestOwnerCannotMarkAFutureDate(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	assertStatus(t, do(t, h, http.MethodPost, "/days/2026-10-15"), http.StatusBadRequest)
	assertRedirect(t, do(t, h, http.MethodPost, "/days/2026-10-14"), "/?month=2026-10")

	got := do(t, h, http.MethodGet, "/export/2026").Body.String()
	if want := "date,weekday\n2026-10-14,Wednesday\n"; got != want {
		t.Errorf("export 2026 = %q, want %q", got, want)
	}
}

func TestInvalidDatesAreRejected(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	for _, target := range []string{
		"/days/2026-02-30",
		"/days/2026-1-5",
		"/days/14.10.2026",
		"/days/today",
	} {
		t.Run(target, func(t *testing.T) {
			assertStatus(t, do(t, h, http.MethodPost, target), http.StatusBadRequest)
		})
	}
}

func TestOwnerUnmarksAWrongDay(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	do(t, h, http.MethodPost, "/days/2026-03-02")
	do(t, h, http.MethodPost, "/days/2026-03-03")

	assertRedirect(t, do(t, h, http.MethodPost, "/days/2026-03-02/delete"), "/?month=2026-03")
	assertRedirect(t, do(t, h, http.MethodPost, "/days/2026-03-04/delete"), "/?month=2026-03")

	got := do(t, h, http.MethodGet, "/export/2026").Body.String()
	if want := "date,weekday\n2026-03-03,Tuesday\n"; got != want {
		t.Errorf("export 2026 = %q, want %q", got, want)
	}
}

func TestUnmarkRejectsInvalidDates(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	for _, target := range []string{"/days/2026-02-30/delete", "/days/today/delete"} {
		t.Run(target, func(t *testing.T) {
			assertStatus(t, do(t, h, http.MethodPost, target), http.StatusBadRequest)
		})
	}
}

func TestMarkedTodayOffersUndo(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	assertNotContains(t, do(t, h, http.MethodGet, "/").Body.String(), "Undo")

	do(t, h, http.MethodPost, "/today")

	body := do(t, h, http.MethodGet, "/").Body.String()
	assertContains(t, body, `<form method="post" action="/days/2026-10-14/delete">`)
	assertContains(t, body, "Undo")
}

var yearCountRow = regexp.MustCompile(`<tr( class="current")?>\s*<td>(\d+)</td>\s*<td>(\d+)</td>\s*<td><a href="/export/(\d+)">CSV</a></td>`)

func yearCountRows(t *testing.T, body string) []string {
	t.Helper()
	var rows []string
	for _, m := range yearCountRow.FindAllStringSubmatch(body, -1) {
		if m[2] != m[4] {
			t.Errorf("row for year %s links to export of year %s", m[2], m[4])
		}
		row := m[2] + "=" + m[3]
		if m[1] != "" {
			row += " current"
		}
		rows = append(rows, row)
	}
	return rows
}

func TestYearCountsShowTheCurrentYearWithZero(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	do(t, h, http.MethodPost, "/days/2025-06-02")

	got := yearCountRows(t, do(t, h, http.MethodGet, "/").Body.String())

	if want := []string{"2026=0 current", "2025=1"}; !slices.Equal(got, want) {
		t.Errorf("year count rows = %v, want %v", got, want)
	}
}

func TestYearCountsListEveryYearWithDataNewestFirst(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	for _, d := range []string{"2019-12-31", "2024-01-01", "2024-12-31", "2026-01-01"} {
		do(t, h, http.MethodPost, "/days/"+d)
	}

	got := yearCountRows(t, do(t, h, http.MethodGet, "/").Body.String())

	if want := []string{"2026=1 current", "2024=2", "2019=1"}; !slices.Equal(got, want) {
		t.Errorf("year count rows = %v, want %v", got, want)
	}
}

func TestExportOfAnEmptyYearHasOnlyTheHeader(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	do(t, h, http.MethodPost, "/days/2026-03-02")

	rec := do(t, h, http.MethodGet, "/export/2020")

	assertStatus(t, rec, http.StatusOK)
	if got, want := rec.Body.String(), "date,weekday\n"; got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}

func TestMonthQueryIsValidated(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	assertStatus(t, do(t, h, http.MethodGet, "/?month=2026-10"), http.StatusOK)
	assertStatus(t, do(t, h, http.MethodGet, "/?month=1999-01"), http.StatusOK)
	assertRedirect(t, do(t, h, http.MethodGet, "/?month=2026-11"), "/")
	for _, month := range []string{"2026-13", "2026-1", "10-2026", "october"} {
		t.Run(month, func(t *testing.T) {
			assertStatus(t, do(t, h, http.MethodGet, "/?month="+month), http.StatusBadRequest)
		})
	}
}

func TestCalendarDaysToggleTheirMark(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	do(t, h, http.MethodPost, "/days/2026-10-05")

	body := do(t, h, http.MethodGet, "/?month=2026-10").Body.String()

	assertContains(t, body, `action="/days/2026-10-05/delete"`)
	assertNotContains(t, body, `action="/days/2026-10-05"`)
	assertContains(t, body, `action="/days/2026-10-06"`)
	assertContains(t, body, `action="/days/2026-10-14"`)
	assertContains(t, body, `action="/days/2026-10-01"`)
	assertNotContains(t, body, `action="/days/2026-09-30"`)
}

var disabledDayButton = regexp.MustCompile(`<button[^>]*\bdisabled\b[^>]*>\s*15\s*</button>`)

func TestCalendarDaysAfterTodayAreDisabled(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	body := do(t, h, http.MethodGet, "/?month=2026-10").Body.String()

	assertNotContains(t, body, `action="/days/2026-10-15"`)
	assertNotContains(t, body, `action="/days/2026-10-31"`)
	if !disabledDayButton.MatchString(body) {
		t.Errorf("no disabled button for 15 October:\n%s", body)
	}
}

func leadingBlankDays(t *testing.T, body string) int {
	t.Helper()
	_, calendar, ok := strings.Cut(body, `<section class="calendar">`)
	if !ok {
		t.Fatalf("no calendar in page:\n%s", body)
	}
	_, rows, _ := strings.Cut(calendar, "<tbody>")
	firstWeek, _, _ := strings.Cut(rows, "</tr>")
	return strings.Count(firstWeek, `<td class="blank">`)
}

func TestCalendarWeeksStartOnMonday(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	for month, want := range map[string]int{"2026-03": 6, "2026-10": 3, "2026-06": 0} {
		t.Run(month, func(t *testing.T) {
			body := do(t, h, http.MethodGet, "/?month="+month).Body.String()
			if got := leadingBlankDays(t, body); got != want {
				t.Errorf("leading blank days = %d, want %d", got, want)
			}
		})
	}
}

func TestMonthNavigationStopsAtTheMonthOfToday(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	current := do(t, h, http.MethodGet, "/").Body.String()
	assertContains(t, current, `href="/?month=2026-09"`)
	assertNotContains(t, current, `href="/?month=2026-11"`)

	previous := do(t, h, http.MethodGet, "/?month=2026-09").Body.String()
	assertContains(t, previous, `href="/?month=2026-08"`)
	assertContains(t, previous, `href="/?month=2026-10"`)

	january := do(t, h, http.MethodGet, "/?month=2026-01").Body.String()
	assertContains(t, january, `href="/?month=2025-12"`)
	assertContains(t, january, `href="/?month=2026-02"`)
}
