package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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
	return newHandlerWithClock(t, func() time.Time { return now })
}

func newHandlerWithClock(t *testing.T, now func() time.Time) http.Handler {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "homeoffice.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return web.NewHandler(s, now, web.ReminderSetup{PublicKey: "test-public-key", SendNow: func() error { return nil }})
}

func do(t *testing.T, h http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func post(t *testing.T, h http.Handler, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func markDays(t *testing.T, h http.Handler, dates ...string) {
	t.Helper()
	assertStatus(t, post(t, h, "/selection", url.Values{"mark": dates}), http.StatusSeeOther)
}

func assertExport(t *testing.T, h http.Handler, year, want string) {
	t.Helper()
	if got := do(t, h, http.MethodGet, "/export/"+year).Body.String(); got != want {
		t.Errorf("export %s = %q, want %q", year, got, want)
	}
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

var berlinSummerTime = time.FixedZone("CEST", 2*60*60)

var wednesdayMorning = time.Date(2026, 10, 14, 10, 0, 0, 0, berlinSummerTime)

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

func postWithHeaders(t *testing.T, h http.Handler, target string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestOtherSitesCannotWrite(t *testing.T) {
	tests := map[string]map[string]string{
		"fetch metadata":         {"Sec-Fetch-Site": "cross-site"},
		"origin over plain http": {"Origin": "http://evil.example"},
	}
	for name, headers := range tests {
		t.Run(name, func(t *testing.T) {
			h := newHandler(t, wednesdayMorning)

			for _, target := range []string{"/today", "/selection", "/reminder/subscriptions", "/reminder/test"} {
				assertStatus(t, postWithHeaders(t, h, target, headers), http.StatusForbidden)
			}

			assertContains(t, do(t, h, http.MethodGet, "/").Body.String(), "Mark today as home office day")
		})
	}
}

func TestOwnBrowserCanWrite(t *testing.T) {
	tests := map[string]map[string]string{
		"fetch metadata":         {"Sec-Fetch-Site": "same-origin"},
		"origin over plain http": {"Origin": "http://example.com"},
	}
	for name, headers := range tests {
		t.Run(name, func(t *testing.T) {
			h := newHandler(t, wednesdayMorning)

			assertRedirect(t, postWithHeaders(t, h, "/today", headers), "/")

			assertContains(t, do(t, h, http.MethodGet, "/").Body.String(), "Today is a home office day")
		})
	}
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

	markDays(t, h, "2026-03-08", "2026-01-05")

	assertExport(t, h, "2026", "date,weekday\n2026-01-05,Monday\n2026-03-08,Sunday\n")
}

func TestTappingAPastDayOnlySelectsIt(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	markDays(t, h, "2026-10-13")
	staleForm := url.Values{"month": {"2026-10"}, "select": {"2026-10-01"}}

	assertRedirect(t, post(t, h, "/days/2026-10-13/delete", staleForm), "/?month=2026-10&select=2026-10-01&select=2026-10-13")
	assertRedirect(t, post(t, h, "/days/2026-10-12", staleForm), "/?month=2026-10&select=2026-10-01&select=2026-10-12")
	assertRedirect(t, post(t, h, "/days/2026-10-01", staleForm), "/?month=2026-10&select=2026-10-01")
	assertRedirect(t, do(t, h, http.MethodPost, "/days/2026-03-08"), "/?month=2026-03&select=2026-03-08")

	assertExport(t, h, "2026", "date,weekday\n2026-10-13,Tuesday\n")
}

func TestTodayCellKeepsTheSelectionAndMonth(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	form := url.Values{"month": {"2026-10"}, "select": {"2026-10-02", "2026-09-30", "2026-10-14", "nonsense"}}
	want := "/?month=2026-10&select=2026-09-30&select=2026-10-02"

	assertRedirect(t, post(t, h, "/days/2026-10-14", form), want)
	assertExport(t, h, "2026", "date,weekday\n2026-10-14,Wednesday\n")

	assertRedirect(t, post(t, h, "/days/2026-10-14/delete", form), want)
	assertExport(t, h, "2026", "date,weekday\n")
}

func TestDayRoutesRejectAnInvalidMonth(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	for _, month := range []string{"2026-13", "october"} {
		t.Run(month, func(t *testing.T) {
			assertStatus(t, post(t, h, "/days/2026-10-14", url.Values{"month": {month}}), http.StatusBadRequest)
		})
	}
	assertExport(t, h, "2026", "date,weekday\n")
}

func TestPostRoutesDropAFutureMonthLikeTheMonthQuery(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	future := url.Values{"month": {"2026-11"}, "select": {"2026-10-02"}}

	assertRedirect(t, post(t, h, "/today", future), "/?select=2026-10-02")
	assertRedirect(t, post(t, h, "/days/2026-10-13", future), "/?month=2026-10&select=2026-10-02&select=2026-10-13")
	assertRedirect(t, post(t, h, "/selection", url.Values{"month": {"2026-11"}, "mark": {"2026-10-02"}}), "/")

	assertExport(t, h, "2026", "date,weekday\n2026-10-02,Friday\n2026-10-14,Wednesday\n")
}

func TestOwnerCannotMarkAFutureDate(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	assertStatus(t, do(t, h, http.MethodPost, "/days/2026-10-15"), http.StatusBadRequest)
	assertStatus(t, do(t, h, http.MethodPost, "/days/2026-10-15/delete"), http.StatusBadRequest)
	assertRedirect(t, do(t, h, http.MethodPost, "/days/2026-10-14"), "/?month=2026-10")

	assertExport(t, h, "2026", "date,weekday\n2026-10-14,Wednesday\n")
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
	markDays(t, h, "2026-03-02", "2026-03-03")

	assertRedirect(t, post(t, h, "/selection", url.Values{"unmark": {"2026-03-02", "2026-03-04"}, "month": {"2026-03"}}), "/?month=2026-03")

	assertExport(t, h, "2026", "date,weekday\n2026-03-03,Tuesday\n")
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
	assertContains(t, body, `<form method="post" action="/today/delete">`)
	assertContains(t, body, "Undo")
}

func TestUndoUnmarksTheTodayOfTheServer(t *testing.T) {
	now := time.Date(2026, 10, 14, 23, 59, 0, 0, berlinSummerTime)
	h := newHandlerWithClock(t, func() time.Time { return now })
	do(t, h, http.MethodPost, "/today")
	assertRedirect(t, do(t, h, http.MethodPost, "/today/delete"), "/")
	do(t, h, http.MethodPost, "/today")

	now = time.Date(2026, 10, 15, 0, 1, 0, 0, berlinSummerTime)
	do(t, h, http.MethodPost, "/today/delete")

	assertExport(t, h, "2026", "date,weekday\n2026-10-14,Wednesday\n")
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
	markDays(t, h, "2025-06-02")

	got := yearCountRows(t, do(t, h, http.MethodGet, "/").Body.String())

	if want := []string{"2026=0 current", "2025=1"}; !slices.Equal(got, want) {
		t.Errorf("year count rows = %v, want %v", got, want)
	}
}

func TestYearCountsListEveryYearWithDataNewestFirst(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	markDays(t, h, "2019-12-31", "2024-01-01", "2024-12-31", "2026-01-01")

	got := yearCountRows(t, do(t, h, http.MethodGet, "/").Body.String())

	if want := []string{"2026=1 current", "2024=2", "2019=1"}; !slices.Equal(got, want) {
		t.Errorf("year count rows = %v, want %v", got, want)
	}
}

func TestExportOfAnEmptyYearHasOnlyTheHeader(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	markDays(t, h, "2026-03-02")

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

type dayCell struct {
	class string
	href  string
}

var dayLinkPattern = regexp.MustCompile(`<a class="([^"]*)" href="([^"]*)">(\d+)</a>`)

func dayLinks(body string) map[string]dayCell {
	links := map[string]dayCell{}
	for _, m := range dayLinkPattern.FindAllStringSubmatch(body, -1) {
		links[m[3]] = dayCell{class: m[1], href: m[2]}
	}
	return links
}

func assertDayLink(t *testing.T, links map[string]dayCell, day string, want dayCell) {
	t.Helper()
	if got, ok := links[day]; !ok || got != want {
		t.Errorf("link for day %s = %+v, want %+v", day, got, want)
	}
}

func TestPastDaysLinkToTheirSelection(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	markDays(t, h, "2026-10-05", "2026-10-06")

	body := do(t, h, http.MethodGet, "/?month=2026-10&select=2026-10-06&select=2026-10-07").Body.String()
	links := dayLinks(body)

	assertDayLink(t, links, "1", dayCell{"day", "/?month=2026-10&amp;select=2026-10-01&amp;select=2026-10-06&amp;select=2026-10-07"})
	assertDayLink(t, links, "5", dayCell{"day marked", "/?month=2026-10&amp;select=2026-10-05&amp;select=2026-10-06&amp;select=2026-10-07"})
	assertDayLink(t, links, "6", dayCell{"day select-unmark", "/?month=2026-10&amp;select=2026-10-07"})
	assertDayLink(t, links, "7", dayCell{"day select-mark", "/?month=2026-10&amp;select=2026-10-06"})
	assertDayLink(t, links, "13", dayCell{"day", "/?month=2026-10&amp;select=2026-10-06&amp;select=2026-10-07&amp;select=2026-10-13"})
	if _, ok := links["14"]; ok {
		t.Errorf("today has a selection link")
	}
	assertNotContains(t, body, `action="/days/2026-10-13"`)
	assertContains(t, body, `action="/days/2026-10-14"`)
}

func TestTodayFormsCarryTheSelectionAndMonth(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	body := do(t, h, http.MethodGet, "/?month=2026-10&select=2026-10-06&select=2026-09-30").Body.String()

	_, todayCell, _ := strings.Cut(body, `action="/days/2026-10-14"`)
	todayCell, _, _ = strings.Cut(todayCell, "</form>")
	_, panel, _ := strings.Cut(body, `action="/today"`)
	panel, _, _ = strings.Cut(panel, "</form>")
	for name, form := range map[string]string{"today cell": todayCell, "today panel": panel} {
		t.Run(name, func(t *testing.T) {
			assertContains(t, form, `<input type="hidden" name="month" value="2026-10">`)
			assertContains(t, form, `<input type="hidden" name="select" value="2026-09-30">`)
			assertContains(t, form, `<input type="hidden" name="select" value="2026-10-06">`)
		})
	}
}

func TestInvalidSelectValuesAreIgnored(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	rec := do(t, h, http.MethodGet, "/?month=2026-10&select=nonsense&select=2026-10-14&select=2026-10-15&select=2026-02-30&select=2026-10-02&select=2026-10-02")

	assertStatus(t, rec, http.StatusOK)
	assertDayLink(t, dayLinks(rec.Body.String()), "1", dayCell{"day", "/?month=2026-10&amp;select=2026-10-01&amp;select=2026-10-02"})
}

func TestMonthNavigationKeepsTheSelection(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	body := do(t, h, http.MethodGet, "/?month=2026-09&select=2026-10-02&select=2026-09-30").Body.String()

	assertContains(t, body, `href="/?month=2026-08&amp;select=2026-09-30&amp;select=2026-10-02"`)
	assertContains(t, body, `href="/?month=2026-10&amp;select=2026-09-30&amp;select=2026-10-02"`)
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

func TestConfirmMarksAndUnmarksAcrossMonths(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	markDays(t, h, "2026-09-30", "2026-10-01")

	rec := post(t, h, "/selection", url.Values{
		"mark":   {"2026-10-05", "2026-09-29", "2026-10-01"},
		"unmark": {"2026-09-30", "2026-10-02"},
		"month":  {"2026-10"},
	})

	assertRedirect(t, rec, "/?month=2026-10")
	assertExport(t, h, "2026", "date,weekday\n2026-09-29,Tuesday\n2026-10-01,Thursday\n2026-10-05,Monday\n")
}

func TestConfirmRejectsAnythingButPastDaysAndWritesNothing(t *testing.T) {
	for name, form := range map[string]url.Values{
		"today":         {"mark": {"2026-10-13", "2026-10-14"}},
		"future":        {"mark": {"2026-10-13", "2026-10-15"}},
		"invalid date":  {"mark": {"2026-10-13"}, "unmark": {"2026-02-30"}},
		"unmark today":  {"mark": {"2026-10-13"}, "unmark": {"2026-10-14"}},
		"in both lists": {"mark": {"2026-10-13", "2026-10-12"}, "unmark": {"2026-10-12"}},
		"invalid month": {"mark": {"2026-10-13"}, "month": {"2026-13"}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newHandler(t, wednesdayMorning)
			markDays(t, h, "2026-10-12")

			assertStatus(t, post(t, h, "/selection", form), http.StatusBadRequest)

			assertExport(t, h, "2026", "date,weekday\n2026-10-12,Monday\n")
		})
	}
}

func TestConfirmWithAnEmptySelectionOnlyRedirects(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	assertRedirect(t, post(t, h, "/selection", url.Values{"month": {"2026-09"}}), "/?month=2026-09")
	assertRedirect(t, post(t, h, "/selection", nil), "/")

	assertExport(t, h, "2026", "date,weekday\n")
}

func TestTodayPanelKeepsTheSelectionAndMonth(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	form := url.Values{"month": {"2026-09"}, "select": {"2026-10-02", "2026-09-30", "2026-10-15"}}
	want := "/?month=2026-09&select=2026-09-30&select=2026-10-02"

	assertRedirect(t, post(t, h, "/today", form), want)
	assertExport(t, h, "2026", "date,weekday\n2026-10-14,Wednesday\n")

	assertRedirect(t, post(t, h, "/today/delete", form), want)
	assertExport(t, h, "2026", "date,weekday\n")

	assertStatus(t, post(t, h, "/today", url.Values{"month": {"2026-13"}}), http.StatusBadRequest)
	assertExport(t, h, "2026", "date,weekday\n")
}

var barSummaryPattern = regexp.MustCompile(`<p class="summary">(.*)</p>`)

func barSummary(t *testing.T, body string) string {
	t.Helper()
	m := barSummaryPattern.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no confirmation bar summary:\n%s", body)
	}
	return m[1]
}

func TestConfirmationBarShowsOnlyWithASelection(t *testing.T) {
	h := newHandler(t, wednesdayMorning)

	for _, target := range []string{"/", "/?month=2026-10&select=2026-10-14&select=2026-10-15"} {
		assertNotContains(t, do(t, h, http.MethodGet, target).Body.String(), `class="confirmation-bar"`)
	}
	assertContains(t, do(t, h, http.MethodGet, "/?select=2026-10-06").Body.String(), `class="confirmation-bar"`)
}

func TestConfirmationBarSummarizesTheSelection(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	markDays(t, h, "2025-12-31", "2026-10-01")

	for name, tc := range map[string]struct{ target, want string }{
		"mark only": {
			"/?select=2026-10-07&select=2026-10-06",
			"Mark 2 days: Tue 6 Oct 2026, Wed 7 Oct 2026",
		},
		"unmark only": {
			"/?select=2026-10-01",
			"Unmark 1 day: <del>Thu 1 Oct 2026</del>",
		},
		"mixed across years": {
			"/?month=2026-10&select=2026-10-07&select=2025-12-31&select=2026-10-01&select=2026-09-30",
			"Mark 2 days, unmark 2 days: <del>Wed 31 Dec 2025</del>, Wed 30 Sep 2026, <del>Thu 1 Oct 2026</del>, Wed 7 Oct 2026",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if got := barSummary(t, do(t, h, http.MethodGet, tc.target).Body.String()); got != tc.want {
				t.Errorf("summary = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConfirmationBarPostsExplicitChanges(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	markDays(t, h, "2025-12-31")

	body := do(t, h, http.MethodGet, "/?month=2026-09&select=2026-10-07&select=2025-12-31").Body.String()
	_, bar, _ := strings.Cut(body, `class="confirmation-bar"`)

	assertContains(t, bar, `action="/selection"`)
	assertContains(t, bar, `<input type="hidden" name="month" value="2026-09">`)
	assertContains(t, bar, `<input type="hidden" name="mark" value="2026-10-07">`)
	assertContains(t, bar, `<input type="hidden" name="unmark" value="2025-12-31">`)
	assertNotContains(t, bar, `name="select"`)
	assertContains(t, bar, `<a class="cancel" href="/?month=2026-09">Cancel</a>`)
}

func TestFaviconsAreServed(t *testing.T) {
	h := newHandler(t, wednesdayMorning)
	for target, contentType := range map[string]string{
		"/favicon.ico":          "image/",
		"/favicon.svg":          "image/svg+xml",
		"/apple-touch-icon.png": "image/png",
	} {
		rec := do(t, h, http.MethodGet, target)
		assertStatus(t, rec, http.StatusOK)
		if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, contentType) {
			t.Errorf("%s Content-Type = %q, want prefix %q", target, got, contentType)
		}
	}
	body := do(t, h, http.MethodGet, "/").Body.String()
	assertContains(t, body, `href="/favicon.svg"`)
	assertContains(t, body, `href="/apple-touch-icon.png"`)
}
