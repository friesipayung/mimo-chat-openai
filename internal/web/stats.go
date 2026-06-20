package web

import (
	"net/http"
	"strconv"

	"github.com/friesipayung/mimo-chat-openai/internal/db"
)

type StatsHandler struct {
	db *db.DB
}

func NewStatsHandler(database *db.DB) *StatsHandler {
	return &StatsHandler{db: database}
}

func (h *StatsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	stats, err := h.db.GetStats()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, stats)
}

type LogsHandler struct {
	db *db.DB
}

func NewLogsHandler(database *db.DB) *LogsHandler {
	return &LogsHandler{db: database}
}

func (h *LogsHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	logs, err := h.db.GetLogs(limit)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if logs == nil {
		logs = []db.RequestLog{}
	}
	writeJSON(w, 200, logs)
}
