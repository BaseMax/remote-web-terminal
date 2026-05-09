package config

import (
	"encoding/json"
	"os"
)

// Config holds application configuration.
type Config struct {
	ListenAddr string `json:"listen_addr"` // e.g. ":8080"
	// Credentials for the web login.
	Username string `json:"username"`
	// Password is stored as bcrypt hash in the config file.
	// Generate with: htpasswd -bnBC 12 "" yourpassword | tr -d ':\n'
	PasswordHash string `json:"password_hash"`
	// Shell to launch, e.g. "/bin/bash" or "/bin/sh"
	Shell string `json:"shell"`
	// ShellArgs are additional args passed to the shell
	ShellArgs []string `json:"shell_args"`
	// SessionSecret is used to sign session cookies (min 32 chars).
	SessionSecret string `json:"session_secret"`
	// MaxSessions limits concurrent terminal sessions.
	MaxSessions int `json:"max_sessions"`
	// IdleTimeoutSeconds: terminal killed after this many seconds of no heartbeat.
	IdleTimeoutSeconds int `json:"idle_timeout_seconds"`
}

// Load reads and parses a config file.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{
		Shell:              "/bin/bash",
		MaxSessions:        10,
		IdleTimeoutSeconds: 300,
	}
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}
