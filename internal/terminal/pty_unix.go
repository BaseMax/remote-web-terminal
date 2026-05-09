//go:build !windows

package terminal

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

type unixPTY struct {
	f   *os.File
	cmd *exec.Cmd
}

// startPTY launches shell under a Unix PTY and returns a ptyConn.
func startPTY(shell string, args []string, cols, rows uint16) (ptyConn, error) {
	cmd := exec.Command(shell, args...)
	cmd.Env = buildEnv(cols, rows)

	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cols, Rows: rows})
	if err != nil {
		return nil, fmt.Errorf("pty start: %w", err)
	}
	return &unixPTY{f: f, cmd: cmd}, nil
}

func (p *unixPTY) Read(b []byte) (int, error)  { return p.f.Read(b) }
func (p *unixPTY) Write(b []byte) (int, error) { return p.f.Write(b) }

// Close closes the PTY file and kills the child process.
func (p *unixPTY) Close() error {
	err := p.f.Close()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Kill()
	}
	return err
}

func (p *unixPTY) resize(cols, rows uint16) error {
	return pty.Setsize(p.f, &pty.Winsize{Cols: cols, Rows: rows})
}

func (p *unixPTY) wait() error { return p.cmd.Wait() }

// buildEnv constructs a clean environment for the child process.
func buildEnv(cols, rows uint16) []string {
	env := []string{
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		fmt.Sprintf("COLUMNS=%d", cols),
		fmt.Sprintf("LINES=%d", rows),
	}
	// Propagate essential variables from the server's environment.
	for _, key := range []string{"HOME", "USER", "LOGNAME", "PATH", "LANG", "LC_ALL", "SHELL", "TZ"} {
		if v := getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}
	return env
}

func getenv(key string) string {
	return os.Getenv(key)
}
