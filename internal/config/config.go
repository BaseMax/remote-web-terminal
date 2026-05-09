package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
)

// Config holds application configuration.
type Config struct {
	ListenAddr string `json:"listen_addr"` // e.g. ":8080"
	// Credentials for the web login.
	Username string `json:"username"`
	// Password is stored as bcrypt hash in the config file.
	// Generate with: go run ./cmd/genhash -password yourpassword
	PasswordHash string `json:"password_hash"`
	// Shell to launch. Leave empty to auto-detect for the host OS.
	Shell string `json:"shell"`
	// ShellArgs are additional args passed to the shell.
	ShellArgs []string `json:"shell_args"`
	// SessionSecret is used to sign session cookies (min 32 chars).
	SessionSecret string `json:"session_secret"`
	// MaxSessions limits concurrent terminal sessions.
	MaxSessions int `json:"max_sessions"`
	// IdleTimeoutSeconds: terminal killed after this many seconds of no heartbeat.
	IdleTimeoutSeconds int `json:"idle_timeout_seconds"`
}

// Load reads and parses a config file, then applies OS-aware defaults.
func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{
		MaxSessions:        10,
		IdleTimeoutSeconds: 300,
	}
	if err := json.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}

	// Auto-detect shell when not set in config.
	if cfg.Shell == "" {
		cfg.Shell = defaultShell()
	}
	if len(cfg.ShellArgs) == 0 {
		cfg.ShellArgs = defaultShellArgs(cfg.Shell)
	}

	return cfg, nil
}

// defaultShell returns the best available interactive shell for the host OS.
func defaultShell() string {
	switch runtime.GOOS {
	case "windows":
		// Prefer PowerShell Core (pwsh), then Windows PowerShell, then cmd.
		for _, candidate := range []string{"pwsh.exe", "powershell.exe", "cmd.exe"} {
			if p, err := exec.LookPath(candidate); err == nil {
				return p
			}
		}
		return "cmd.exe"

	default:
		// Honour the user's $SHELL if set.
		if sh := os.Getenv("SHELL"); sh != "" {
			return sh
		}
		// Fall back by OS family.
		switch runtime.GOOS {
		case "freebsd", "openbsd", "netbsd", "dragonfly":
			if p, err := exec.LookPath("bash"); err == nil {
				return p
			}
			return "/bin/sh"
		default: // linux, darwin
			if p, err := exec.LookPath("bash"); err == nil {
				return p
			}
			return "/bin/sh"
		}
	}
}

// defaultShellArgs returns sensible default arguments for well-known shells.
func defaultShellArgs(shell string) []string {
	// Use only the base name for matching so absolute paths work too.
	base := shell
	for i := len(shell) - 1; i >= 0; i-- {
		if shell[i] == '/' || shell[i] == '\\' {
			base = shell[i+1:]
			break
		}
	}

	switch base {
	case "powershell.exe", "powershell":
		return []string{"-NoLogo"}
	case "pwsh.exe", "pwsh":
		return []string{"-NoLogo"}
	default:
		return []string{}
	}
}
