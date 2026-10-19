package web

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
)

func (s *server) export(w http.ResponseWriter, r *http.Request) {
	year, err := strconv.Atoi(r.PathValue("year"))
	if err != nil {
		http.Error(w, "invalid year", http.StatusBadRequest)
		return
	}
	days, err := s.store.Days(r.Context(), year)
	if err != nil {
		serverError(w, r, err)
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
