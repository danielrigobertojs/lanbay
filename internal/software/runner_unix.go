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

func runCommandImpl(ctx context.Context, path string, args []string, env []string, dir string, out io.Writer) (int, []byte, error) {
	var buf bytes.Buffer
	w := io.MultiWriter(out, &cappedWriter{buf: &buf, cap: 1 << 20})
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
	if err != nil {
		return code, buf.Bytes(), err
	}
	return code, buf.Bytes(), nil
}

type cappedWriter struct {
	buf *bytes.Buffer
	cap int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if c.buf.Len() < c.cap {
		remaining := c.cap - c.buf.Len()
		if len(p) > remaining {
			c.buf.Write(p[:remaining])
		} else {
			c.buf.Write(p)
		}
	}
	return len(p), nil
}

// runCommand is a package-level variable so tests can replace it.
var runCommand = runCommandImpl
