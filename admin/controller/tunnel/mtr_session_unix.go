//go:build !windows

package tunnel

import (
	"context"
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

func startMTRSession(ctx context.Context, mtrPath string, args []string) (*os.File, *exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, mtrPath, args...)
	cmd.Env = append(os.Environ(),
		"TERM=xterm",
		"LINES=40",
		"COLUMNS=160",
	)

	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, nil, err
	}
	defer tty.Close()

	if err := pty.Setsize(ptmx, &pty.Winsize{
		Rows: 40,
		Cols: 160,
	}); err != nil {
		ptmx.Close()
		return nil, nil, err
	}

	cmd.Stdin = tty
	cmd.Stdout = tty
	cmd.Stderr = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: true,
		Ctty:    0,
	}

	if err := cmd.Start(); err != nil {
		ptmx.Close()
		return nil, nil, err
	}
	return ptmx, cmd, nil
}
