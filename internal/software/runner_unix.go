// SPDX-License-Identifier: Apache-2.0
//go:build !windows

package software

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
)

func init() { _ = runCommand }

func runCommand(ctx context.Context, path string, args []string, env []string, dir string, out io.Writer) (int, []byte, error) {
	var buf bytes.Buffer
	w := io.MultiWriter(out, &buf)
	cmd := exec.Command(path, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Env = env
	cmd.Dir = dir
	if err := cmd.Start(); err != nil {
		return 0, buf.Bytes(), fmt.Errorf("start command: %w", err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var code int
	var err error
	select {
	case waitErr := <-done:
		if waitErr != nil {
			if exit, ok := waitErr.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				err = waitErr
			}
		}
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		err = ctx.Err()
		code = -1
	}
	if err != nil && code != -1 {
		// process ran but failed to wait? keep code 0
	}
	if err != nil {
		return code, buf.Bytes(), err
	}
	return code, buf.Bytes(), nil
}
