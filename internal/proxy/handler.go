package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/friesipayung/mimo-chat-openai/internal/db"
	"github.com/friesipayung/mimo-chat-openai/internal/mimo"
	"github.com/friesipayung/mimo-chat-openai/internal/types"
)

type Handler struct {
	db     *db.DB
	client *mimo.Client
}

func NewHandler(database *db.DB) *Handler {
	return &Handler{
		db:     database,
		client: mimo.NewClient(),
	}
}

func (h *Handler) HandleModels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(types.ModelList{Object: "list", Data: types.SupportedModels})
}

// RequireAPIKey middleware - validates API key from Authorization header
func (h *Handler) RequireAPIKey(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract API key from Authorization: Bearer <key>
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			writeError(w, http.StatusUnauthorized, "missing Authorization header. Use: Authorization: Bearer <api_key>")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			writeError(w, http.StatusUnauthorized, "invalid Authorization format. Use: Authorization: Bearer <api_key>")
			return
		}

		apiKey := parts[1]
		key, err := h.db.GetAPIKeyByKey(apiKey)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "invalid API key")
			return
		}

		// Store key name and key value for logging and usage tracking
		r.Header.Set("X-API-Key-Name", key.Name)
		r.Header.Set("X-API-Key", apiKey)
		next(w, r)
	}
}

func (h *Handler) HandleChat(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	apiKeyName := r.Header.Get("X-API-Key-Name")
	apiKey := r.Header.Get("X-API-Key")

	var req types.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	cookie, err := h.db.GetRandomCookie()
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, "no available cookies")
		return
	}

	mimoModel := types.ModelIDToStudio(req.Model)
	query := buildQuery(req.Messages)

	temp := 0.8
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	topP := 0.95
	if req.TopP != nil {
		topP = *req.TopP
	}

	// Determine thinking mode
	// Default: enabled for all models except mimo-v2-flash
	enableThinking := mimoModel != "mimo-v2-flash-studio" && mimoModel != "mimo-v2-omni"
	if req.Thinking != nil {
		enableThinking = req.Thinking.Type == "enabled"
	}

	mimoReq := mimo.MiMoRequest{
		MsgID:          mimo.RandHex(16),
		ConversationID: mimo.RandHex(16),
		Query:          query,
		IsEditedQuery:  false,
		ModelConfig: mimo.MiMoModelCfg{
			EnableThinking:  enableThinking,
			WebSearchStatus: "disabled",
			Model:           mimoModel,
			Temperature:     temp,
			TopP:            topP,
		},
		MultiMedias: []interface{}{},
	}

	resp, err := h.client.Chat(cookie.FullCookie, mimoReq)
	if err != nil {
		h.logRequest(cookie, apiKeyName, req.Model, 0, 0, 0, err.Error(), start)
		writeError(w, http.StatusBadGateway, "upstream error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		h.logRequest(cookie, apiKeyName, req.Model, 0, 0, resp.StatusCode, "upstream status error", start)
		writeError(w, http.StatusBadGateway, "upstream returned status "+resp.Status)
		return
	}

	if req.Stream {
		h.handleStream(w, resp, req.Model, cookie, apiKeyName, apiKey, start)
	} else {
		h.handleSync(w, resp, req.Model, cookie, apiKeyName, apiKey, start)
	}
}

func (h *Handler) handleStream(w http.ResponseWriter, resp *http.Response, model string, cookie *db.Cookie, apiKeyName, apiKey string, start time.Time) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	mimo.StreamToOpenAI(resp.Body, model, w, func() { flusher.Flush() })

	h.db.UpdateCookieUsage(cookie.ID, 0)
	h.db.UpdateAPIKeyUsage(apiKey)
	h.logRequest(cookie, apiKeyName, model, 0, 0, 200, "", start)
}

func (h *Handler) handleSync(w http.ResponseWriter, resp *http.Response, model string, cookie *db.Cookie, apiKeyName, apiKey string, start time.Time) {
	result, err := mimo.CollectSync(resp.Body, model)
	if err != nil {
		h.logRequest(cookie, apiKeyName, model, 0, 0, 502, err.Error(), start)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	if result.Usage != nil {
		h.db.UpdateCookieUsage(cookie.ID, result.Usage.TotalTokens)
		h.db.UpdateAPIKeyUsage(apiKey)
		h.logRequest(cookie, apiKeyName, model, result.Usage.PromptTokens, result.Usage.CompletionTokens, 200, "", start)
	} else {
		h.db.UpdateCookieUsage(cookie.ID, 0)
		h.db.UpdateAPIKeyUsage(apiKey)
		h.logRequest(cookie, apiKeyName, model, 0, 0, 200, "", start)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (h *Handler) logRequest(cookie *db.Cookie, apiKeyName, model string, promptTokens, completionTokens, statusCode int, errMsg string, start time.Time) {
	alias := ""
	if cookie != nil {
		alias = cookie.Alias
	}
	var cookieID *int64
	if cookie != nil {
		cookieID = &cookie.ID
	}
	h.db.AddLog(&db.RequestLog{
		CookieID:         cookieID,
		CookieAlias:      alias,
		APIKeyName:       apiKeyName,
		Model:            model,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		StatusCode:       statusCode,
		Error:            errMsg,
		DurationMs:       int(time.Since(start).Milliseconds()),
	})
}

func buildQuery(messages []types.ChatMessage) string {
	var parts []string
	for _, m := range messages {
		text := types.ExtractText(m.Content)
		switch m.Role {
		case "system":
			parts = append(parts, "System: "+text)
		case "user":
			parts = append(parts, "Human: "+text)
		case "assistant":
			parts = append(parts, "Assistant: "+text)
		default:
			parts = append(parts, m.Role+": "+text)
		}
	}
	return strings.Join(parts, "\n")
}

func writeError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(types.ErrorResponse{
		Error: types.ErrorBody{Message: message, Type: "invalid_request_error"},
	})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
