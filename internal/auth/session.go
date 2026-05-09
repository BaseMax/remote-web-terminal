package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

type Session struct {
	ID        string
	Username  string
	CreatedAt time.Time
	LastSeen  time.Time
}

type SessionStore struct {
	mu      sync.RWMutex
	store   map[string]*Session
	ttl     time.Duration
	secret  []byte
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	s := &SessionStore{
		store:  make(map[string]*Session),
		ttl:    ttl,
		secret: make([]byte, 32),
	}
	_, _ = rand.Read(s.secret)
	go s.gcLoop()
	return s
}

func (s *SessionStore) SetSecret(secret []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secret = secret
}

func (s *SessionStore) Create(username string) string {
	id := randomHex(32)
	sig := s.sign(id)
	token := id + "." + sig

	sess := &Session{
		ID:        id,
		Username:  username,
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}
	s.mu.Lock()
	s.store[id] = sess
	s.mu.Unlock()
	return token
}

func (s *SessionStore) Validate(token string) (*Session, bool) {
	if len(token) < 65 {
		return nil, false
	}
	dot := len(token) - 65 // sha256 hex = 64
	if dot <= 0 || token[dot] != '.' {
		return nil, false
	}
	id := token[:dot]
	sig := token[dot+1:]

	expected := s.sign(id)
	if !hmac.Equal([]byte(sig), []byte(expected)) {
		return nil, false
	}

	s.mu.RLock()
	sess, ok := s.store[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}

	s.mu.Lock()
	if time.Since(sess.LastSeen) > s.ttl {
		delete(s.store, id)
		s.mu.Unlock()
		return nil, false
	}
	sess.LastSeen = time.Now()
	s.mu.Unlock()
	return sess, true
}

func (s *SessionStore) Delete(token string) {
	if len(token) < 65 {
		return
	}
	dot := len(token) - 65
	if dot <= 0 || token[dot] != '.' {
		return
	}
	id := token[:dot]
	s.mu.Lock()
	delete(s.store, id)
	s.mu.Unlock()
}

func (s *SessionStore) sign(id string) string {
	s.mu.RLock()
	secret := s.secret
	s.mu.RUnlock()
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(id))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *SessionStore) gcLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		for id, sess := range s.store {
			if time.Since(sess.LastSeen) > s.ttl {
				delete(s.store, id)
			}
		}
		s.mu.Unlock()
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const cookieName = "wt_session"

func SetCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		// Secure:   true, // enable when behind HTTPS/nginx
	})
}

func ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func TokenFromRequest(r *http.Request) string {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return ""
	}
	return c.Value
}
