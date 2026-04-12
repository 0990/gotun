//go:build windows

package tunnel

import (
	"context"
	"errors"
	"os"
	"os/exec"
)

func startMTRSession(ctx context.Context, mtrPath string, args []string) (*os.File, *exec.Cmd, error) {
	return nil, nil, errors.New("pty-backed mtr stream is unsupported on windows build host")
}
