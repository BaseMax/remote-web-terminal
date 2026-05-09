package terminal

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/basemax/remote-web-terminal/internal/config"
)

// POST /api/terminal/create
// Body: { "cols": 80, "rows": 24 }
func CreateHandler(w http.ResponseWriter, r *http.Request, m *Manager, cfg *config.Config) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Cols uint16 `json:"cols"`
		Rows uint16 `json:"rows"`
	}
	req.Cols = 80
	req.Rows = 24

	if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
		if req.Cols == 0 {
			req.Cols = 80
		}
		if req.Rows == 0 {
			req.Rows = 24
		}
	}

	idleTimeout := time.Duration(cfg.IdleTimeoutSeconds) * time.Second

	sess, err := m.Create(cfg.Shell, cfg.ShellArgs, req.Cols, req.Rows, idleTimeout)
	if err != nil {
		jsonError(w, "failed to start terminal: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"session_id": sess.ID,
	})
}

// POST /api/terminal/input
// Body: { "session_id": "...", "data": "<base64>" }
func InputHandler(w http.ResponseWriter, r *http.Request, m *Manager) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Data      string `json:"data"` // base64-encoded bytes
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	sess, ok := m.Get(req.SessionID)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	raw, err := base64.StdEncoding.DecodeString(req.Data)
	if err != nil {
		jsonError(w, "invalid base64 data", http.StatusBadRequest)
		return
	}

	if len(raw) > 64*1024 {
		jsonError(w, "input too large", http.StatusRequestEntityTooLarge)
		return
	}

	if err := sess.Write(raw); err != nil {
		jsonError(w, "write failed: "+err.Error(), http.StatusGone)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// GET /api/terminal/output?session_id=...&offset=...
// Uses long-polling
func OutputHandler(w http.ResponseWriter, r *http.Request, m *Manager) {
	if r.Method != http.MethodGet {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionID := r.URL.Query().Get("session_id")
	offsetStr := r.URL.Query().Get("offset")

	offset, _ := strconv.Atoi(offsetStr)
	if offset < 0 {
		offset = 0
	}

	sess, ok := m.Get(sessionID)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	deadline := time.Now().Add(20 * time.Second)
	pollInterval := 30 * time.Millisecond

	for {
		data, nextOffset, closed := sess.ReadOutput(offset)
		if len(data) > 0 || closed {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        base64.StdEncoding.EncodeToString(data),
				"next_offset": nextOffset,
				"closed":      closed,
			})
			return
		}

		if time.Now().After(deadline) {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data":        "",
				"next_offset": nextOffset,
				"closed":      false,
			})
			return
		}

		time.Sleep(pollInterval)
		if pollInterval < 100*time.Millisecond {
			pollInterval += 10 * time.Millisecond
		}
	}
}

// POST /api/terminal/resize
// Body: { "session_id": "...", "cols": 80, "rows": 24 }
func ResizeHandler(w http.ResponseWriter, r *http.Request, m *Manager) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
		Cols      uint16 `json:"cols"`
		Rows      uint16 `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	sess, ok := m.Get(req.SessionID)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	if req.Cols == 0 {
		req.Cols = 80
	}
	if req.Rows == 0 {
		req.Rows = 24
	}

	if err := sess.Resize(req.Cols, req.Rows); err != nil {
		jsonError(w, "resize failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/terminal/close
func CloseHandler(w http.ResponseWriter, r *http.Request, m *Manager) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	m.Close(req.SessionID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// POST /api/terminal/heartbeat
func HeartbeatHandler(w http.ResponseWriter, r *http.Request, m *Manager) {
	if r.Method != http.MethodPost {
		jsonError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	_, ok := m.Get(req.SessionID)
	if !ok {
		jsonError(w, "session not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
