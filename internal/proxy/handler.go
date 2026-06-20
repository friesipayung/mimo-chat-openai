package proxy

import (
	"encoding/json"
	"fmt"
	"io"
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

	temp := 0.8
	if req.Temperature != nil {
		temp = *req.Temperature
	}
	topP := 0.95
	if req.TopP != nil {
		topP = *req.TopP
	}

	enableThinking := mimoModel != "mimo-v2-flash-studio" && mimoModel != "mimo-v2-omni"
	if req.Thinking != nil {
		enableThinking = req.Thinking.Type == "enabled"
	}

	// Process messages for multimodal content
	query, medias, err := h.processMessages(cookie.FullCookie, req.Messages, mimoModel)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error processing messages: "+err.Error())
		return
	}

	if medias == nil {
		medias = []interface{}{}
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
		MultiMedias: medias,
	}

	resp, err := h.client.Chat(cookie.FullCookie, mimoReq)
	if err != nil {
		h.logRequest(cookie, apiKeyName, req.Model, 0, 0, 0, err.Error(), start)
		writeError(w, http.StatusBadGateway, "upstream error: "+err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		h.logRequest(cookie, apiKeyName, req.Model, 0, 0, resp.StatusCode, string(bodyBytes), start)
		writeError(w, http.StatusBadGateway, fmt.Sprintf("upstream returned status %d: %s", resp.StatusCode, string(bodyBytes)))
		return
	}

	if req.Stream {
		h.handleStream(w, resp, req.Model, cookie, apiKeyName, apiKey, start)
	} else {
		h.handleSync(w, resp, req.Model, cookie, apiKeyName, apiKey, start)
	}
}

// processMessages handles multimodal content and returns the query string and media items
func (h *Handler) processMessages(cookie string, messages []types.ChatMessage, model string) (string, []interface{}, error) {
	var parts []string
	var medias []interface{}

	for _, msg := range messages {
		content := msg.Content
		text := ""

		// Check if content is a multipart array
		if contentArr, ok := content.([]interface{}); ok {
			for _, item := range contentArr {
				contentMap, ok := item.(map[string]interface{})
				if !ok {
					continue
				}

				contentType, _ := contentMap["type"].(string)

				switch contentType {
				case "text":
					if t, ok := contentMap["text"].(string); ok {
						text += t
					}

				case "image_url":
					if imageURL, ok := contentMap["image_url"].(map[string]interface{}); ok {
						if urlStr, ok := imageURL["url"].(string); ok {
							media, err := h.processMediaURL(cookie, urlStr, "image", model)
							if err != nil {
								return "", nil, fmt.Errorf("error processing image: %w", err)
							}
							if media != nil {
								medias = append(medias, media)
							}
						}
					}

				case "input_audio":
					if audioData, ok := contentMap["input_audio"].(map[string]interface{}); ok {
						if urlStr, ok := audioData["data"].(string); ok {
							media, err := h.processMediaURL(cookie, urlStr, "audio", model)
							if err != nil {
								return "", nil, fmt.Errorf("error processing audio: %w", err)
							}
							if media != nil {
								medias = append(medias, media)
							}
						}
					}

				case "video_url":
					if videoURL, ok := contentMap["video_url"].(map[string]interface{}); ok {
						if urlStr, ok := videoURL["url"].(string); ok {
							media, err := h.processMediaURL(cookie, urlStr, "video", model)
							if err != nil {
								return "", nil, fmt.Errorf("error processing video: %w", err)
							}
							if media != nil {
								medias = append(medias, media)
							}
						}
					}
				}
			}
		} else {
			// Simple string content
			text = types.ExtractText(content)
		}

		if text != "" {
			switch msg.Role {
			case "system":
				parts = append(parts, "System: "+text)
			case "user":
				parts = append(parts, "Human: "+text)
			case "assistant":
				parts = append(parts, "Assistant: "+text)
			default:
				parts = append(parts, msg.Role+": "+text)
			}
		}
	}

	return strings.Join(parts, "\n"), medias, nil
}

// processMediaURL handles a media URL (either base64 or HTTP URL)
func (h *Handler) processMediaURL(cookie string, urlStr string, mediaType string, model string) (interface{}, error) {
	if strings.HasPrefix(urlStr, "data:") {
		// Base64 encoded data - upload to FDS
		data, mime, err := mimo.ParseBase64Image(urlStr)
		if err != nil {
			return nil, err
		}

		ext := mimo.GetMimeExt(mime)
		fileName := mimo.RandHex(8) + ext
		detectedType := mimo.GetMediaTypeFromMime(mime)

		media, err := h.client.UploadFile(cookie, data, fileName, detectedType, model)
		if err != nil {
			return nil, err
		}

		return media, nil
	}

	// HTTP URL - download and upload to FDS
	data, fileName, err := h.downloadFile(urlStr)
	if err != nil {
		return nil, fmt.Errorf("error downloading file: %w", err)
	}

	media, err := h.client.UploadFile(cookie, data, fileName, mediaType, model)
	if err != nil {
		return nil, err
	}

	return media, nil
}

// downloadFile downloads a file from a URL and returns the data and filename
func (h *Handler) downloadFile(urlStr string) ([]byte, string, error) {
	resp, err := http.Get(urlStr)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// Extract filename from URL or generate one
	fileName := ""
	parts := strings.Split(urlStr, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		if idx := strings.Index(lastPart, "?"); idx > 0 {
			lastPart = lastPart[:idx]
		}
		if lastPart != "" {
			fileName = lastPart
		}
	}
	if fileName == "" {
		fileName = mimo.RandHex(8) + ".bin"
	}

	return data, fileName, nil
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
