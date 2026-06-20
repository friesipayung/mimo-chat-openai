package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/friesipayung/mimo-chat-openai/internal/db"
	"github.com/friesipayung/mimo-chat-openai/internal/mimo"
)

type CookieHandler struct {
	db     *db.DB
	client *mimo.Client
}

func NewCookieHandler(database *db.DB) *CookieHandler {
	return &CookieHandler{
		db:     database,
		client: mimo.NewClient(),
	}
}

type AddCookieRequest struct {
	Alias        string `json:"alias"`
	ServiceToken string `json:"service_token"`
	UserID       string `json:"user_id"`
	Ph           string `json:"ph"`
	FullCookie   string `json:"full_cookie"`
}

type BulkCookieRequest struct {
	Cookies string `json:"cookies"`
}

func (h *CookieHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	cookies, err := h.db.ListCookies()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if cookies == nil {
		cookies = []db.Cookie{}
	}
	writeJSON(w, 200, cookies)
}

func (h *CookieHandler) HandleAdd(w http.ResponseWriter, r *http.Request) {
	var req AddCookieRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	if req.Alias == "" {
		writeJSON(w, 400, map[string]string{"error": "alias is required"})
		return
	}

	// Input validation
	if len(req.Alias) > 100 {
		writeJSON(w, 400, map[string]string{"error": "alias too long (max 100 chars)"})
		return
	}
	if len(req.FullCookie) > 10000 {
		writeJSON(w, 400, map[string]string{"error": "cookie too long (max 10000 chars)"})
		return
	}

	if req.FullCookie != "" {
		st, uid, ph := mimo.ParseCookieParts(req.FullCookie)
		req.ServiceToken = st
		req.UserID = uid
		req.Ph = ph
	}

	if req.ServiceToken == "" || req.UserID == "" || req.Ph == "" {
		writeJSON(w, 400, map[string]string{"error": "incomplete cookie data"})
		return
	}

	if req.FullCookie == "" {
		req.FullCookie = mimo.BuildCookie(req.ServiceToken, req.UserID, req.Ph)
	}

	id, err := h.db.AddCookie(req.Alias, req.ServiceToken, req.UserID, req.Ph, req.FullCookie)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]interface{}{"id": id, "alias": req.Alias})
}

func (h *CookieHandler) HandleBulkAdd(w http.ResponseWriter, r *http.Request) {
	var req BulkCookieRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	lines := strings.Split(req.Cookies, "\n")
	var added []map[string]interface{}
	var errors []string

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		st, uid, ph := mimo.ParseCookieParts(line)
		if st == "" || uid == "" || ph == "" {
			errors = append(errors, "line "+strconv.Itoa(i+1)+": incomplete cookie")
			continue
		}

		alias := "cookie-" + strconv.Itoa(len(added)+1)
		id, err := h.db.AddCookie(alias, st, uid, ph, line)
		if err != nil {
			errors = append(errors, "line "+strconv.Itoa(i+1)+": "+err.Error())
			continue
		}

		added = append(added, map[string]interface{}{"id": id, "alias": alias})
	}

	writeJSON(w, 200, map[string]interface{}{"added": added, "errors": errors})
}

func (h *CookieHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
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

	if err := h.db.DeleteCookie(id); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "deleted"})
}

func (h *CookieHandler) HandleHealthCheck(w http.ResponseWriter, r *http.Request) {
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

	cookie, err := h.db.GetCookie(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "cookie not found"})
		return
	}

	go func() {
		valid, status := h.client.HealthCheck(cookie.FullCookie)
		newStatus := "unknown"
		if valid {
			newStatus = "valid"
		} else {
			newStatus = status
		}
		h.db.UpdateCookieStatus(id, newStatus)
	}()

	writeJSON(w, 200, map[string]string{"status": "checking"})
}

func (h *CookieHandler) HandleCheckAll(w http.ResponseWriter, r *http.Request) {
	cookies, err := h.db.ListCookies()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	for _, c := range cookies {
		go func(cookie db.Cookie) {
			valid, status := h.client.HealthCheck(cookie.FullCookie)
			newStatus := "unknown"
			if valid {
				newStatus = "valid"
			} else {
				newStatus = status
			}
			h.db.UpdateCookieStatus(cookie.ID, newStatus)
		}(c)
	}

	writeJSON(w, 200, map[string]string{"status": "checking all"})
}

func (h *CookieHandler) HandleToggle(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	enabledStr := r.URL.Query().Get("enabled")

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid id"})
		return
	}

	enabled := enabledStr == "true"
	if err := h.db.ToggleCookie(id, enabled); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (h *CookieHandler) HandleUpdateAlias(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ID    int64  `json:"id"`
		Alias string `json:"alias"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	if req.Alias == "" {
		writeJSON(w, 400, map[string]string{"error": "alias is required"})
		return
	}

	if err := h.db.UpdateCookieAlias(req.ID, req.Alias); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, 200, map[string]string{"status": "updated"})
}

func (h *CookieHandler) HandleCheckBalance(w http.ResponseWriter, r *http.Request) {
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

	cookie, err := h.db.GetCookie(id)
	if err != nil {
		writeJSON(w, 404, map[string]string{"error": "cookie not found"})
		return
	}

	go func() {
		balanceResp, err := h.client.GetBalance(cookie.FullCookie)
		if err != nil {
			h.db.UpdateCookieBalance(id, "error", "")
			return
		}
		if balanceResp.Code == 0 && balanceResp.Data.Balance != "" {
			h.db.UpdateCookieBalance(id, balanceResp.Data.Balance, balanceResp.Data.Currency)
		} else {
			h.db.UpdateCookieBalance(id, "N/A", "")
		}
	}()

	writeJSON(w, 200, map[string]string{"status": "checking"})
}

func (h *CookieHandler) HandleCheckBalanceAll(w http.ResponseWriter, r *http.Request) {
	cookies, err := h.db.ListCookies()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}

	for _, c := range cookies {
		go func(cookie db.Cookie) {
			balanceResp, err := h.client.GetBalance(cookie.FullCookie)
			if err != nil {
				h.db.UpdateCookieBalance(cookie.ID, "error", "")
				return
			}
			if balanceResp.Code == 0 && balanceResp.Data.Balance != "" {
				h.db.UpdateCookieBalance(cookie.ID, balanceResp.Data.Balance, balanceResp.Data.Currency)
			} else {
				h.db.UpdateCookieBalance(cookie.ID, "N/A", "")
			}
		}(c)
	}

	writeJSON(w, 200, map[string]string{"status": "checking all"})
}
