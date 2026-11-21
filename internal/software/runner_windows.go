// SPDX-License-Identifier: Apache-2.0
//go:build windows

package software

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"
)

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
	job, _ := newJobObject()
	if job != nil {
		_ = job.assign(uint32(cmd.Process.Pid))
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if job != nil {
			job.close()
		}
		code := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				return 0, buf.Bytes(), err
			}
		}
		return code, buf.Bytes(), nil
	case <-ctx.Done():
		if job != nil {
			_ = job.terminate()
		} else if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		if job != nil {
			job.close()
		}
		return -1, buf.Bytes(), ctx.Err()
	}
}
