//go:build windows

package terminal

import (
	"context"
	"fmt"
	"strings"
	"syscall"

	"github.com/UserExistsError/conpty"
)

type winPTY struct {
	c *conpty.ConPty
}

// startPTY launches shell under a Windows ConPTY and returns a ptyConn.
func startPTY(shell string, args []string, cols, rows uint16) (ptyConn, error) {
	cmdLine := buildCmdLine(shell, args)

	c, err := conpty.Start(cmdLine,
		conpty.ConPtyDimensions(int(cols), int(rows)),
	)
	if err != nil {
		return nil, fmt.Errorf("conpty start: %w", err)
	}
	return &winPTY{c: c}, nil
}

// buildCmdLine assembles a properly-quoted Windows command line.
func buildCmdLine(shell string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, syscall.EscapeArg(shell))
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}

func (p *winPTY) Read(b []byte) (int, error)  { return p.c.Read(b) }
func (p *winPTY) Write(b []byte) (int, error) { return p.c.Write(b) }
func (p *winPTY) Close() error                { return p.c.Close() }

func (p *winPTY) resize(cols, rows uint16) error {
	return p.c.Resize(int(cols), int(rows))
}

func (p *winPTY) wait() error {
	_, err := p.c.Wait(context.Background())
	return err
}
