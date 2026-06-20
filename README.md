# mimo-chat-openai

> **EXPERIMENTAL & EDUCATIONAL PURPOSE ONLY**

Xiaomi MiMo Web → OpenAI-compatible API proxy with built-in Web UI management.

## Disclaimer

- This project is for **learning and research purposes only**
- It reverse-engineers the Xiaomi MiMo Studio web interface (`aistudio.xiaomimimo.com`)
- **Not affiliated with Xiaomi** in any way
- **Not for commercial use**
- Use at your own risk — may violate Xiaomi's Terms of Service
- No warranty, no support, no liability

## What It Does

Wraps the Xiaomi MiMo Studio web chat API into a standard OpenAI Chat Completions compatible endpoint, with a Web UI for cookie management, API key management, monitoring, and configuration.

```
┌─────────────────┐      ┌──────────────────┐      ┌─────────────────────────┐
│  Client (curl,  │ ──── │  mimo-chat-openai│ ──── │  aistudio.xiaomimimo.com│
│  OpenAI SDK)    │      │  :8090           │      │  /open-apis/bot/chat    │
└─────────────────┘      └──────────────────┘      └─────────────────────────┘
   OpenAI format            Cookie-based auth          Xiaomi Web API (SSE)
```

## Features

### API

- OpenAI-compatible `/v1/chat/completions` endpoint
- **API key authentication** — Multiple keys with enable/disable toggle
- Streaming & non-streaming support
- **Thinking/reasoning** — Chain-of-thought extraction via `reasoning_content`
- Model selection — 5 MiMo models available
- Configurable temperature, top-p parameters

### Web UI

- Dashboard with request stats, token usage, success rate
- Cookie management (per-key & bulk input)
- **Balance checking** — View account balance per cookie
- API key management (create, delete, enable/disable)
- Cookie health check (auto-detect expired cookies)
- Request logging with API key and cookie tracking
- Model configuration (default model, temperature, top-p, thinking mode)
- Password change

### Infrastructure

- Multi-account cookie pool with random selection
- SQLite storage (cookies, API keys, config, logs)
- Single binary deployment
- Docker Compose ready

## Supported Models

| Model ID | MiMo Model | Context | Default Thinking |
|----------|------------|---------|------------------|
| `mimo-v2.5-pro` | MiMo-V2.5-Pro | 1M | Enabled |
| `mimo-v2.5` | MiMo-V2.5 | 1M | Enabled |
| `mimo-v2-flash` | MiMo-V2-Flash | 256K | Disabled |
| `mimo-v2-pro` | MiMo-V2-Pro | 1M | Enabled |
| `mimo-v2-omni` | MiMo-V2-Omni | 256K | Enabled |

## Quick Start

### Docker Compose (Recommended)

```bash
git clone git@github.com:friesipayung/mimo-chat-openai.git
cd mimo-chat-openai
docker compose up -d
```

Open `http://localhost:8090/ui/` for Web UI.

### From Source

```bash
go build ./cmd/mimo-proxy/
./mimo-proxy
```

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `LISTEN` | `:8090` | Listen address |
| `ADMIN_PASSWORD` | `12345678` | Initial Web UI password (can be changed in UI) |
| `DB_PATH` | `data/mimo.db` | SQLite database path |

## How to Get Cookies

There are two types of cookies needed:

1. **Chat Cookies** — Required for API requests (`/v1/chat/completions`)
2. **Platform Cookies** — Optional, for balance checking

### Method 1: Get Chat Cookies (Required)

1. Open https://aistudio.xiaomimimo.com/ and login with Xiaomi account
2. Open DevTools (F12) → Network tab
3. Send any message in the chat
4. Find the `/open-apis/bot/chat` request
5. Copy the full `Cookie` header value

**Cookie format:**
```
serviceToken="xxx"; userId=123456; xiaomichatbot_ph="xxx"
```

**Required fields:**
- `serviceToken` — Session token from Xiaomi login
- `userId` — Numeric user ID
- `xiaomichatbot_ph` — Fingerprint hash

### Method 2: Get Platform Cookies (For Balance)

To check account balance, you need additional platform cookies:

1. Open https://platform.xiaomimimo.com/console/balance and login
2. Open DevTools (F12) → Network tab
3. Refresh the page or wait for balance to load
4. Find the `/api/v1/balance` request
5. Copy the full `Cookie` header value

**Cookie format (with platform cookies):**
```
api-platform_serviceToken="xxx"; api-platform_slh="xxx"; api-platform_ph="xxx"; serviceToken="xxx"; userId=123456; xiaomichatbot_ph="xxx"
```

**Additional fields for balance:**
- `api-platform_serviceToken` — Platform session token
- `api-platform_slh` — Platform security hash
- `api-platform_ph` — Platform fingerprint

### Method 3: Quick Copy (All Cookies)

To get all cookies at once:

1. Login to both:
   - https://aistudio.xiaomimimo.com/
   - https://platform.xiaomimimo.com/
2. Open DevTools (F12) → Application → Cookies
3. For domain `xiaomimimo.com`, copy all cookie values
4. Format as: `key1="value1"; key2="value2"; ...`

### Cookie Examples

**Chat only (no balance):**
```
serviceToken="abc123..."; userId=6877411486; xiaomichatbot_ph="xyz789..."
```

**With balance checking:**
```
api-platform_serviceToken="def456..."; api-platform_slh="ghi789..."; api-platform_ph="jkl012..."; serviceToken="abc123..."; userId=6877411486; xiaomichatbot_ph="xyz789..."
```

## Usage

### 1. Create API Key

First, create an API key via Web UI (http://localhost:8090/ui/#/keys) or via API:

```bash
# Login to get session cookie
curl -X POST http://localhost:8090/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"password":"12345678"}' \
  -c cookies.txt

# Create API key
curl -X POST http://localhost:8090/api/keys \
  -H "Content-Type: application/json" \
  -b cookies.txt \
  -d '{"name":"my-key"}'

# Response: {"id":1, "key":"sk-xxxx...", "name":"my-key"}
```

### 2. Chat Completion (Non-streaming)

```bash
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx..." \
  -d '{
    "model": "mimo-v2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": false
  }'
```

### 3. Chat Completion (Streaming)

```bash
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx..." \
  -d '{
    "model": "mimo-v2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": true
  }'
```

### 4. With Thinking/Reasoning

```bash
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx..." \
  -d '{
    "model": "mimo-v2.5-pro",
    "messages": [{"role": "user", "content": "What is 15 * 23? Think step by step."}],
    "thinking": {"type": "enabled"}
  }'
```

Response includes `reasoning_content`:

```json
{
  "choices": [{
    "message": {
      "role": "assistant",
      "content": "15 × 23 = 345",
      "reasoning_content": "First, I need to compute 15 * 23..."
    }
  }]
}
```

### 5. Image Understanding (Multimodal)

```bash
# With image URL
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx..." \
  -d '{
    "model": "mimo-v2.5",
    "messages": [{
      "role": "user",
      "content": [
        {"type": "image_url", "image_url": {"url": "https://example.com/image.jpg"}},
        {"type": "text", "text": "Describe this image"}
      ]
    }]
  }'

# With base64 image
curl -X POST http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer sk-xxxx..." \
  -d '{
    "model": "mimo-v2.5",
    "messages": [{
      "role": "user",
      "content": [
        {"type": "image_url", "image_url": {"url": "data:image/jpeg;base64,/9j/4AAQ..."}},
        {"type": "text", "text": "What is in this image?"}
      ]
    }]
  }'
```

**Supported models for multimodal:** `mimo-v2.5`, `mimo-v2-omni`

**Supported media types:**
- Images: JPEG, PNG, GIF, WebP, BMP (max 50MB)
- Audio: MP3, WAV, FLAC, M4A, OGG (max 100MB)
- Video: MP4, MOV, AVI, WMV (max 300MB)

### 6. OpenAI Python SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8090/v1",
    api_key="sk-xxxx..."
)

# Simple chat
response = client.chat.completions.create(
    model="mimo-v2.5-pro",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)

# With thinking
response = client.chat.completions.create(
    model="mimo-v2.5-pro",
    messages=[{"role": "user", "content": "Explain quantum computing"}],
    extra_body={"thinking": {"type": "enabled"}}
)
print(response.choices[0].message.reasoning_content)
print(response.choices[0].message.content)

# With image
response = client.chat.completions.create(
    model="mimo-v2.5",
    messages=[{
        "role": "user",
        "content": [
            {"type": "image_url", "image_url": {"url": "https://example.com/photo.jpg"}},
            {"type": "text", "text": "What's in this image?"}
        ]
    }]
)
print(response.choices[0].message.content)
```

### 7. List Models

```bash
curl http://localhost:8090/v1/models
```

## API Reference

### Endpoints

| Endpoint | Method | Auth | Description |
|----------|--------|------|-------------|
| `/v1/chat/completions` | POST | API Key | Chat completion |
| `/v1/models` | GET | None | List models |
| `/health` | GET | None | Health check |

### Request Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `model` | string | required | Model ID (e.g. `mimo-v2.5-pro`) |
| `messages` | array | required | Chat messages |
| `stream` | boolean | `false` | Enable streaming |
| `thinking` | object | `null` | Thinking config `{"type": "enabled"}` or `{"type": "disabled"}` |
| `temperature` | number | `0.8` | Sampling temperature (0-2) |
| `top_p` | number | `0.95` | Top-p sampling (0-1) |

### Authentication

API requests require `Authorization: Bearer <api_key>` header.

API keys can be created and managed via:
- Web UI: http://localhost:8090/ui/#/keys
- API: `POST /api/keys` (requires session auth)

## Web UI

After starting, open `http://localhost:8090/ui/`:

| Page | Description |
|------|-------------|
| **Dashboard** | Request stats, token usage, success rate, API usage info |
| **Cookies** | Add/delete cookies, bulk input, health check, balance check |
| **API Keys** | Create/delete keys, enable/disable toggle, usage tracking |
| **Configuration** | Default model, temperature, top-p, thinking mode, password change |
| **Logs** | Recent request logs with API key and cookie info |

Default password: `12345678` (can be changed in Configuration page)

### Cookie Balance

To check balance, cookies must include platform tokens (see [How to Get Cookies](#method-2-get-platform-cookies-for-balance)).

Click the wallet icon 💳 next to each cookie or "Check Balance" button to fetch balance.

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go (net/http) |
| Frontend | Vue 3 SPA (embedded) |
| Database | SQLite (modernc.org/sqlite) |
| Styling | Tailwind CSS |
| Build | Docker multi-stage |

## Project Structure

```
mimo-chat-openai/
├── cmd/
│   └── mimo-proxy/
│       ├── main.go
│       └── web/dist/index.html    # Embedded Vue SPA
├── internal/
│   ├── config/config.go
│   ├── db/db.go                   # SQLite storage
│   ├── mimo/
│   │   ├── client.go              # MiMo web API client
│   │   └── sse.go                 # SSE parser
│   ├── proxy/handler.go           # OpenAI proxy handler
│   └── web/
│       ├── auth.go                # Session auth
│       ├── apikeys.go             # API key & password handlers
│       ├── cookies.go
│       ├── config.go
│       └── stats.go
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

## License

MIT — For educational purposes only.

## Related Projects

- [rong6/mimo-2api](https://github.com/rong6/mimo-2api) — Similar proxy in Go
- [tcsenpai/mimoapi](https://github.com/tcsenpai/mimoapi) — TypeScript SDK
- [mark-618/mimo-auth](https://github.com/mark-618/mimo-auth) — Profile switcher
- [xyuai/XiaomiMiMo-TUI](https://github.com/xyuai/XiaomiMiMo-TUI) — Terminal client

## Legal

This project is not endorsed by, directly affiliated with, maintained, authorized, or sponsored by Xiaomi. All product names, logos, and brands are property of their respective owners.
