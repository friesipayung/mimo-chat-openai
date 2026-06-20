package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/friesipayung/mimo-chat-openai/internal/config"
	"github.com/friesipayung/mimo-chat-openai/internal/db"
	"github.com/friesipayung/mimo-chat-openai/internal/proxy"
	"github.com/friesipayung/mimo-chat-openai/internal/web"
)

//go:embed web/dist
var webDist embed.FS

func main() {
	cfg := config.Load()

	database, err := db.Open(cfg.DBPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer database.Close()

	log.Printf("Database opened at %s", cfg.DBPath)

	// Load password from database
	dbPassword, err := database.GetPassword()
	if err != nil {
		log.Printf("Warning: could not load password from db, using default: %v", err)
		dbPassword = cfg.AdminPassword
	}

	proxyHandler := proxy.NewHandler(database)
	sessionStore := web.NewSessionStore(dbPassword)
	authHandler := web.NewAuthHandler(sessionStore)
	cookieHandler := web.NewCookieHandler(database)
	configHandler := web.NewConfigHandler(database)
	statsHandler := web.NewStatsHandler(database)
	logsHandler := web.NewLogsHandler(database)
	apiKeyHandler := web.NewAPIKeyHandler(database)
	passwordHandler := web.NewPasswordHandler(database)

	mux := http.NewServeMux()

	// Public API endpoints
	mux.HandleFunc("/v1/models", proxyHandler.HandleModels)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	// LLMs.txt - API documentation for AI agents
	mux.HandleFunc("/llms.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		content := strings.ReplaceAll(llmsTxt, "http://localhost:8090", cfg.GetBaseURL())
		w.Write([]byte(content))
	})

	// Protected API endpoint (API key required or static token)
	if cfg.HasAPIToken() {
		// If static token is set, use it for authentication
		mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+cfg.APIToken {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(401)
				w.Write([]byte(`{"error":{"message":"unauthorized","type":"invalid_request_error"}}`))
				return
			}
			r.Header.Set("X-API-Key-Name", "static-token")
			r.Header.Set("X-API-Key", cfg.APIToken)
			proxyHandler.HandleChat(w, r)
		})
	} else {
		// Otherwise require API key from database
		mux.HandleFunc("/v1/chat/completions", proxyHandler.RequireAPIKey(proxyHandler.HandleChat))
	}

	// Auth endpoints
	mux.HandleFunc("/api/auth/login", authHandler.HandleLogin)
	mux.HandleFunc("/api/auth/logout", authHandler.HandleLogout)
	mux.HandleFunc("/api/auth/status", authHandler.HandleStatus)

	// Cookie management (session protected)
	mux.HandleFunc("/api/cookies", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			authHandler.RequireAuth(cookieHandler.HandleList)(w, r)
		case "POST":
			authHandler.RequireAuth(cookieHandler.HandleAdd)(w, r)
		case "DELETE":
			authHandler.RequireAuth(cookieHandler.HandleDelete)(w, r)
		default:
			http.Error(w, "Method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/cookies/bulk", authHandler.RequireAuth(cookieHandler.HandleBulkAdd))
	mux.HandleFunc("/api/cookies/health", authHandler.RequireAuth(cookieHandler.HandleHealthCheck))
	mux.HandleFunc("/api/cookies/health-all", authHandler.RequireAuth(cookieHandler.HandleCheckAll))
	mux.HandleFunc("/api/cookies/toggle", authHandler.RequireAuth(cookieHandler.HandleToggle))
	mux.HandleFunc("/api/cookies/alias", authHandler.RequireAuth(cookieHandler.HandleUpdateAlias))
	mux.HandleFunc("/api/cookies/balance", authHandler.RequireAuth(cookieHandler.HandleCheckBalance))
	mux.HandleFunc("/api/cookies/balance-all", authHandler.RequireAuth(cookieHandler.HandleCheckBalanceAll))

	// API key management (session protected)
	mux.HandleFunc("/api/keys", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			authHandler.RequireAuth(apiKeyHandler.HandleList)(w, r)
		case "POST":
			authHandler.RequireAuth(apiKeyHandler.HandleAdd)(w, r)
		case "DELETE":
			authHandler.RequireAuth(apiKeyHandler.HandleDelete)(w, r)
		default:
			http.Error(w, "Method not allowed", 405)
		}
	})
	mux.HandleFunc("/api/keys/toggle", authHandler.RequireAuth(apiKeyHandler.HandleToggle))

	// Config management (session protected)
	mux.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			authHandler.RequireAuth(configHandler.HandleGet)(w, r)
		case "PUT":
			authHandler.RequireAuth(configHandler.HandleUpdate)(w, r)
		default:
			http.Error(w, "Method not allowed", 405)
		}
	})

	// Password management (session protected)
	mux.HandleFunc("/api/password", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", 405)
			return
		}
		authHandler.RequireAuth(passwordHandler.HandleChange)(w, r)
	})

	// Stats & Logs (session protected)
	mux.HandleFunc("/api/stats", authHandler.RequireAuth(statsHandler.HandleGet))
	mux.HandleFunc("/api/logs", authHandler.RequireAuth(logsHandler.HandleGet))

	// Serve Vue SPA
	webFS, err := fs.Sub(webDist, "web/dist")
	if err != nil {
		log.Printf("Warning: web/dist not found, web UI disabled: %v", err)
	} else {
		fileServer := http.FileServer(http.FS(webFS))
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			if path == "/" {
				http.Redirect(w, r, "/ui/", http.StatusFound)
				return
			}
			if strings.HasPrefix(path, "/ui") {
				r.URL.Path = strings.TrimPrefix(path, "/ui")
				if r.URL.Path == "" {
					r.URL.Path = "/"
				}
				fileServer.ServeHTTP(w, r)
				return
			}
			fileServer.ServeHTTP(w, r)
		})
	}

	// Add security headers middleware
	handler := addSecurityHeaders(mux)

	log.Printf("Starting mimo-chat-openai on %s", cfg.ListenAddr)
	log.Printf("Web UI: http://localhost%s/ui/", cfg.ListenAddr)
	log.Printf("API: http://localhost%s/v1/chat/completions", cfg.ListenAddr)

	if err := http.ListenAndServe(cfg.ListenAddr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func addSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy - allow CDN for Tailwind, Vue, fonts
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' 'unsafe-eval' https://cdn.tailwindcss.com https://unpkg.com; " +
			"style-src 'self' 'unsafe-inline' https://cdn.tailwindcss.com https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com; " +
			"img-src 'self' data: https:; " +
			"connect-src 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		next.ServeHTTP(w, r)
	})
}

const llmsTxt = `# mimo-chat-openai - OpenAI-Compatible API Proxy

> Xiaomi MiMo Web → OpenAI-compatible API proxy with Web UI management.

## Base URL

http://localhost:8090

## Authentication

All API requests require API key in header:
Authorization: Bearer <api_key>

Create API keys via Web UI at /ui/#/keys or via POST /api/keys

## Endpoints

### POST /v1/chat/completions
OpenAI-compatible chat completion endpoint.

Request body:
{
  "model": "mimo-v2.5-pro",
  "messages": [
    {"role": "system", "content": "You are a helpful assistant."},
    {"role": "user", "content": "Hello!"}
  ],
  "stream": false,
  "temperature": 0.8,
  "top_p": 0.95,
  "thinking": {"type": "enabled"},
  "tools": [
    {"type": "web_search", "web_search": {"enabled": true}}
  ]
}

Response (non-streaming):
{
  "id": "chatcmpl-xxx",
  "object": "chat.completion",
  "created": 1234567890,
  "model": "mimo-v2.5-pro",
  "choices": [{
    "index": 0,
    "message": {
      "role": "assistant",
      "content": "Hello! How can I help you?",
      "reasoning_content": "The user said hello..."
    },
    "finish_reason": "stop"
  }],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 20,
    "total_tokens": 30
  }
}

Response (streaming):
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","choices":[{"delta":{"role":"assistant"}}]}
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","choices":[{"delta":{"content":"Hello"}}]}
data: {"id":"chatcmpl-xxx","object":"chat.completion.chunk","choices":[{"delta":{},"finish_reason":"stop"}]}
data: [DONE]

### GET /v1/models
List available models.

Response:
{
  "object": "list",
  "data": [
    {"id": "mimo-v2.5-pro", "object": "model", "owned_by": "xiaomi"},
    {"id": "mimo-v2.5", "object": "model", "owned_by": "xiaomi"},
    {"id": "mimo-v2-flash", "object": "model", "owned_by": "xiaomi"},
    {"id": "mimo-v2-pro", "object": "model", "owned_by": "xiaomi"},
    {"id": "mimo-v2-omni", "object": "model", "owned_by": "xiaomi"}
  ]
}

### GET /health
Health check endpoint.

Response: {"status": "ok"}

## Features

### Thinking/Reasoning
Enable chain-of-thought reasoning:
{
  "model": "mimo-v2.5-pro",
  "messages": [...],
  "thinking": {"type": "enabled"}
}

Reasoning content returned in response.choices[0].message.reasoning_content

Default behavior:
- mimo-v2.5-pro, mimo-v2.5, mimo-v2-pro, mimo-v2-omni: thinking enabled
- mimo-v2-flash: thinking disabled

### Web Search
Enable real-time web search:
{
  "model": "mimo-v2.5-pro",
  "messages": [...],
  "tools": [{"type": "web_search", "web_search": {"enabled": true}}]
}

Supported models: all

### Structured Output (JSON Mode)
Force model to respond with valid JSON:
{
  "model": "mimo-v2.5-pro",
  "messages": [...],
  "response_format": {"type": "json_object"}
}

The response content will be valid JSON.

### Multimodal (Image/Audio/Video)
Send images, audio, or video in messages:

Image (URL):
{
  "model": "mimo-v2.5",
  "messages": [{
    "role": "user",
    "content": [
      {"type": "image_url", "image_url": {"url": "https://example.com/img.jpg"}},
      {"type": "text", "text": "Describe this image"}
    ]
  }]
}

Image (Base64):
{
  "model": "mimo-v2.5",
  "messages": [{
    "role": "user",
    "content": [
      {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQ..."}},
      {"type": "text", "text": "What is this?"}
    ]
  }]
}

Audio:
{
  "model": "mimo-v2.5",
  "messages": [{
    "role": "user",
    "content": [
      {"type": "input_audio", "input_audio": {"data": "https://example.com/audio.wav"}},
      {"type": "text", "text": "Transcribe this audio"}
    ]
  }]
}

Video:
{
  "model": "mimo-v2.5",
  "messages": [{
    "role": "user",
    "content": [
      {"type": "video_url", "video_url": {"url": "https://example.com/video.mp4"}},
      {"type": "text", "text": "Describe this video"}
    ]
  }]
}

Supported models for multimodal: mimo-v2.5, mimo-v2-omni
Supported formats:
- Images: JPEG, PNG, GIF, WebP, BMP (max 50MB)
- Audio: MP3, WAV, FLAC, M4A, OGG (max 100MB)
- Video: MP4, MOV, AVI, WMV (max 300MB)

## Models

| Model | Context | Default Thinking | Multimodal |
|-------|---------|------------------|------------|
| mimo-v2.5-pro | 1M | Enabled | No |
| mimo-v2.5 | 1M | Enabled | Yes |
| mimo-v2-flash | 256K | Disabled | No |
| mimo-v2-pro | 1M | Enabled | No |
| mimo-v2-omni | 256K | Enabled | Yes |

## Request Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| model | string | required | Model ID |
| messages | array | required | Chat messages |
| stream | boolean | false | Enable streaming |
| temperature | number | 0.8 | Sampling temperature (0-2) |
| top_p | number | 0.95 | Top-p sampling (0-1) |
| max_tokens | int | null | Max output tokens |
| thinking | object | null | Thinking config {"type": "enabled"} |
| tools | array | null | Tools (web_search) |
| response_format | object | null | Response format {"type": "json_object"} |

## Web UI

Access at: http://localhost:8090/ui/

Features:
- Dashboard: Request stats, token usage, success rate
- Cookies: Manage Xiaomi account cookies, balance check
- API Keys: Create/delete/enable/disable keys
- Configuration: Default model, temperature, thinking, password
- Logs: Request history with API key and cookie info

Default password: 12345678 (change in Configuration)

## Error Codes

| Code | Description |
|------|-------------|
| 400 | Invalid request |
| 401 | Missing or invalid API key |
| 403 | Forbidden |
| 404 | Not found |
| 429 | Rate limit exceeded |
| 500 | Internal server error |
| 502 | Upstream error |
| 503 | No available cookies |

## Notes

- This is an EXPERIMENTAL & EDUCATIONAL proxy
- Uses Xiaomi MiMo web interface (cookie-based auth)
- Not affiliated with Xiaomi
- Cookies may expire and need manual refresh
- Balance check requires platform cookies

Source: https://github.com/friesipayung/mimo-chat-openai`
