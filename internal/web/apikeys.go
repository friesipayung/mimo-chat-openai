package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/friesipayung/mimo-chat-openai/internal/db"
)

type APIKeyHandler struct {
	db *db.DB
}

func NewAPIKeyHandler(database *db.DB) *APIKeyHandler {
	return &APIKeyHandler{db: database}
}

type AddAPIKeyRequest struct {
	Name string `json:"name"`
}

func generateKey() string {
	b := make([]byte, 32)
	rand.Read(b)
	return "sk-" + hex.EncodeToString(b)
}

func (h *APIKeyHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	keys, err := h.db.ListAPIKeys()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, keys)
}

func (h *APIKeyHandler) HandleAdd(w http.ResponseWriter, r *http.Request) {
	var req AddAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	if req.Name == "" {
		writeJSON(w, 400, map[string]string{"error": "name is required"})
		return
	}

	key := generateKey()
	id, err := h.db.AddAPIKey(req.Name, key)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]interface{}{
		"id":   id,
		"name": req.Name,
		"key":  key,
	})
}

func (h *APIKeyHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeJSON(w, 400, map[string]string{"error": "id is required"})
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid id"})
		return
	}

	if err := h.db.DeleteAPIKey(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (h *APIKeyHandler) HandleToggle(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	enabledStr := r.URL.Query().Get("enabled")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid id"})
		return
	}

	enabled := enabledStr == "true"
	if err := h.db.ToggleAPIKey(id, enabled); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

type PasswordHandler struct {
	db *db.DB
}

func NewPasswordHandler(database *db.DB) *PasswordHandler {
	return &PasswordHandler{db: database}
}

type ChangePasswordRequest struct {
	OldPassword string `json:"old_password"`
	NewPassword string `json:"new_password"`
}

func (h *PasswordHandler) HandleChange(w http.ResponseWriter, r *http.Request) {
	var req ChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	currentPassword, err := h.db.GetPassword()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	if req.OldPassword != currentPassword {
		writeJSON(w, 401, map[string]string{"error": "incorrect current password"})
		return
	}

	if len(req.NewPassword) < 6 {
		writeJSON(w, 400, map[string]string{"error": "password must be at least 6 characters"})
		return
	}

	if err := h.db.SetPassword(req.NewPassword); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "password changed"})
}
