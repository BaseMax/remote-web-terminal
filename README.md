# Remote Web Terminal

A secure, Go-powered web terminal over plain HTTP/AJAX (no WebSockets).  
Uses long-polling for output streaming and xterm.js for a fully interactive terminal experience.

## Features

- **Secure session auth** — bcrypt password verification, HMAC-signed session cookies, HttpOnly/SameSite flags
- **Interactive terminal** — xterm.js with full PTY support: colors, cursor, interactive apps (vim, htop, ssh, etc.)
- **Copy / Paste** — `Ctrl+Shift+C` to copy selection, `Ctrl+Shift+V` to paste
- **Multi-tab** — open multiple terminal sessions simultaneously
- **Long-poll AJAX** — no WebSocket, works through any HTTP proxy/nginx
- **Auto-resize** — terminal adapts to window/pane size automatically
- **Idle cleanup** — inactive sessions are automatically reaped server-side
- **Nginx-ready** — includes example nginx config with SSL, security headers, and correct timeouts

## Quick Start

### 1. Generate a password hash

```bash
go run ./cmd/genhash -password 'your-secure-password'
```

Copy the output hash.

### 2. Edit `config.json`

```json
{
  "listen_addr": ":8080",
  "username": "admin",
  "password_hash": "<paste bcrypt hash here>",
  "shell": "/bin/bash",
  "shell_args": [],
  "session_secret": "a-random-string-at-least-32-chars-long",
  "max_sessions": 10,
  "idle_timeout_seconds": 300
}
```

### 3. Build

```bash
go mod tidy
go build -o remote-web-terminal .
```

### 4. Run

```bash
./remote-web-terminal -config config.json
```

Open `http://localhost:8080` — you will be redirected to `/login`.

## Nginx Setup

See [`nginx.conf.example`](nginx.conf.example).  
Key point: `/api/terminal/output` needs `proxy_read_timeout 35s` because the server long-polls up to 20 seconds.

## API Reference

All API endpoints require a valid session cookie (set by `/login`).

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/login` | Authenticate (JSON or form) |
| `GET`  | `/logout` | Invalidate session |
| `POST` | `/api/terminal/create` | Start new PTY session → `{ session_id }` |
| `POST` | `/api/terminal/input` | Send input → `{ session_id, data: base64 }` |
| `GET`  | `/api/terminal/output` | Long-poll output → `{ data: base64, next_offset, closed }` |
| `POST` | `/api/terminal/resize` | Resize PTY → `{ session_id, cols, rows }` |
| `POST` | `/api/terminal/close` | Close session |
| `POST` | `/api/terminal/heartbeat` | Keep session alive |

## Security Notes

- Passwords stored as bcrypt hashes (cost 12 default)
- Session tokens are HMAC-SHA256 signed to prevent forgery
- Cookies are `HttpOnly`, `SameSite=Strict`; enable `Secure` flag when behind HTTPS/nginx
- Rate-limiting and IP allowlisting should be done at the nginx layer

## Project Structure

```
.
├── main.go
├── config.json
├── go.mod
├── cmd/genhash/         # bcrypt hash generator tool
├── internal/
│   ├── auth/            # Session store, HMAC tokens, login/logout handlers, middleware
│   ├── config/          # Config loader
│   └── terminal/        # PTY manager, HTTP handlers
├── web/
│   ├── index.html       # Terminal UI
│   ├── login.html       # Login page
│   └── static/          # CSS + JS
└── nginx.conf.example
```

License MIT

Copyright 2026, Seyyed Ali Mohammadiyeh (MAX BASE)
