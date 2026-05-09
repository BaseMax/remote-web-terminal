package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/basemax/remote-web-terminal/internal/config"
	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 30 * time.Minute

// LoginHandler handles GET (show form) and POST (authenticate).
func LoginHandler(w http.ResponseWriter, r *http.Request, cfg *config.Config, sessions *SessionStore) {
	switch r.Method {
	case http.MethodGet:
		http.ServeFile(w, r, "./web/login.html")
	case http.MethodPost:
		handleLoginPost(w, r, cfg, sessions)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleLoginPost(w http.ResponseWriter, r *http.Request, cfg *config.Config, sessions *SessionStore) {
	// Accept both JSON and form-encoded bodies
	var username, password string

	ct := r.Header.Get("Content-Type")
	if strings.Contains(ct, "application/json") {
		var body struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "invalid request body", http.StatusBadRequest)
			return
		}
		username = body.Username
		password = body.Password
	} else {
		if err := r.ParseForm(); err != nil {
			jsonError(w, "invalid form data", http.StatusBadRequest)
			return
		}
		username = r.FormValue("username")
		password = r.FormValue("password")
	}

	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		jsonError(w, "username and password required", http.StatusBadRequest)
		return
	}

	// Constant-time username comparison to prevent timing attacks
	if !hmacEqual(username, cfg.Username) {
		log.Printf("auth: failed login attempt for user %q from %s", username, r.RemoteAddr)
		time.Sleep(500 * time.Millisecond) // slow brute-force
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(password)); err != nil {
		log.Printf("auth: failed login attempt for user %q from %s", username, r.RemoteAddr)
		time.Sleep(500 * time.Millisecond)
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token := sessions.Create(username)
	SetCookie(w, token, sessionTTL)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// LogoutHandler clears the session.
func LogoutHandler(w http.ResponseWriter, r *http.Request, sessions *SessionStore) {
	token := TokenFromRequest(r)
	if token != "" {
		sessions.Delete(token)
	}
	ClearCookie(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// Middleware returns an HTTP middleware that enforces authentication.
func Middleware(sessions *SessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := TokenFromRequest(r)
			if _, ok := sessions.Validate(token); !ok {
				// For API paths return JSON error, otherwise redirect
				if strings.HasPrefix(r.URL.Path, "/api/") {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
					return
				}
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// hmacEqual does a constant-time string comparison.
func hmacEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	result := byte(0)
	for i := 0; i < len(a); i++ {
		result |= a[i] ^ b[i]
	}
	return result == 0
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
