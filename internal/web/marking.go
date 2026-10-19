package web

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"homeoffice-tracker/internal/civil"
)

func (s *server) markToday(w http.ResponseWriter, r *http.Request) {
	s.changeToday(w, r, s.store.Mark)
}

func (s *server) unmarkToday(w http.ResponseWriter, r *http.Request) {
	s.changeToday(w, r, s.store.Unmark)
}

func (s *server) changeToday(w http.ResponseWriter, r *http.Request, write func(context.Context, civil.Date) error) {
	today := s.today()
	state, err := parsePageForm(r, today)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := write(r.Context(), today); err != nil {
		serverError(w, r, err)
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

func (s *server) changeTodayCell(w http.ResponseWriter, r *http.Request, write func(context.Context, civil.Date) error) {
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
		if err := write(r.Context(), day); err != nil {
			serverError(w, r, err)
			return
		}
	} else {
		state.selection = state.selection.with(day)
	}
	http.Redirect(w, r, state.url(), http.StatusSeeOther)
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
		if err := s.store.Apply(r.Context(), mark, unmark); err != nil {
			serverError(w, r, err)
			return
		}
	}
	http.Redirect(w, r, pageState{month: state.month}.url(), http.StatusSeeOther)
}
