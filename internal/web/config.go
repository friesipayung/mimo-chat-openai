package web

import (
	"encoding/json"
	"net/http"

	"github.com/friesipayung/mimo-chat-openai/internal/db"
)

type ConfigHandler struct {
	db *db.DB
}

func NewConfigHandler(database *db.DB) *ConfigHandler {
	return &ConfigHandler{db: database}
}

func (h *ConfigHandler) HandleGet(w http.ResponseWriter, r *http.Request) {
	config, err := h.db.GetAllConfig()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	defaults := map[string]string{
		"default_model":    "mimo-v2.5-pro",
		"temperature":      "0.8",
		"top_p":            "0.95",
		"enable_thinking":  "true",
		"save_history":     "true",
		"max_retries":      "3",
	}

	for k, v := range defaults {
		if _, ok := config[k]; !ok {
			config[k] = v
		}
	}

	writeJSON(w, 200, config)
}

func (h *ConfigHandler) HandleUpdate(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	allowed := map[string]bool{
		"default_model":   true,
		"temperature":     true,
		"top_p":           true,
		"enable_thinking": true,
		"save_history":    true,
		"max_retries":     true,
	}

	for k, v := range req {
		if !allowed[k] {
			continue
		}
		if err := h.db.SetConfig(k, v); err != nil {
			writeJSON(w, 500, map[string]string{"error": err.Error()})
			return
		}
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}
