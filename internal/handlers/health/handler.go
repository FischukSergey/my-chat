// Package health содержит HTTP-обработчики проверки состояния сервиса.
package health

import (
	"encoding/json"
	"net/http"
)

type response struct {
	Status string `json:"status"`
}

// Handle возвращает статус доступности сервиса.
func Handle(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := response{Status: "ok"}

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
}
