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

Wraps the Xiaomi MiMo Studio web chat API into a standard OpenAI Chat Completions compatible endpoint, with a Web UI for cookie management, monitoring, and configuration.

```
┌─────────────────┐      ┌──────────────────┐      ┌─────────────────────────┐
│  Client (curl,  │ ──── │  mimo-chat-openai│ ──── │  aistudio.xiaomimimo.com│
│  OpenAI SDK)    │      │  :8090           │      │  /open-apis/bot/chat    │
└─────────────────┘      └──────────────────┘      └─────────────────────────┘
   OpenAI format            Cookie-based auth          Xiaomi Web API (SSE)
```

## Features

- OpenAI-compatible `/v1/chat/completions` endpoint
- Streaming & non-streaming support
- Thinking/reasoning content extraction
- Multi-account cookie pool with random selection
- Web UI for cookie management (per-key & bulk input)
- Cookie health check (auto-detect expired cookies)
- Dashboard with request stats, token usage, success rate
- Request logging
- Model configuration (temperature, top-p, default model)
- Single binary deployment
- Docker Compose ready

## Supported Models

| Model ID | MiMo Model | Context |
|----------|------------|---------|
| `mimo-v2.5-pro` | MiMo-V2.5-Pro | 1M |
| `mimo-v2.5` | MiMo-V2.5 | 1M |
| `mimo-v2-flash` | MiMo-V2-Flash | 256K |
| `mimo-v2-pro` | MiMo-V2-Pro | 1M |
| `mimo-v2-omni` | MiMo-V2-Omni | 256K |

## Quick Start

### Docker Compose (Recommended)

```bash
git clone git@github.com:friesipayung/mimo-chat-openai.git
cd mimo-chat-openai
docker compose up -d
```

Open `http://localhost:8090` for Web UI.

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `ADMIN_PASSWORD` | `12345678` | Web UI login password |
| `LISTEN` | `:8090` | Listen address |
| `DB_PATH` | `/data/mimo.db` | SQLite database path |

## How to Get Cookies

1. Open https://aistudio.xiaomimimo.com/ and login with Xiaomi account
2. Open DevTools (F12) → Network tab
3. Send any message in the chat
4. Find the `/open-apis/bot/chat` request
5. Copy the full `Cookie` header value

## Usage

### curl

```bash
curl http://localhost:8090/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "mimo-v2.5-pro",
    "messages": [{"role": "user", "content": "Hello!"}],
    "stream": false
  }'
```

### OpenAI Python SDK

```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8090/v1",
    api_key="not-needed"
)

response = client.chat.completions.create(
    model="mimo-v2.5-pro",
    messages=[{"role": "user", "content": "Hello!"}]
)
print(response.choices[0].message.content)
```

## Web UI

After starting, open `http://localhost:8090`:

- **Dashboard** — Request stats, token usage, success rate
- **Cookies** — Add/edit/delete cookies, health check
- **Config** — Default model, temperature, thinking mode
- **Logs** — Recent request logs

Default password: `12345678`

## Tech Stack

| Component | Technology |
|-----------|-----------|
| Backend | Go (net/http) |
| Frontend | Vue 3 SPA |
| Database | SQLite |
| Build | Docker multi-stage |

## Project Structure

```
mimo-chat-openai/
├── cmd/
│   └── mimo-proxy/
│       └── main.go
├── internal/
│   ├── config/
│   ├── db/
│   ├── mimo/
│   ├── proxy/
│   └── web/
├── web/                  # Vue SPA source
├── migrations/
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

## Legal

This project is not endorsed by, directly affiliated with, maintained, authorized, or sponsored by Xiaomi. All product names, logos, and brands are property of their respective owners.
