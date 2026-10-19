package web

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"time"

	"homeoffice-tracker/internal/civil"
)

const monthLayout = "2006-01"

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
