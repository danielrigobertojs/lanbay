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

	"golang.org/x/sys/windows"
)

const createSuspended = 0x00000004

func runCommand(ctx context.Context, path string, args []string, env []string, dir string, out io.Writer) (int, []byte, error) {
	var buf bytes.Buffer
	w := io.MultiWriter(out, &cappedWriter{buf: &buf, cap: 1 << 20})
	cmd := exec.Command(path, args...)
	cmd.Stdout = w
	cmd.Stderr = w
	cmd.Env = env
	cmd.Dir = dir
	cmd.SysProcAttr = &windows.SysProcAttr{CreationFlags: createSuspended}
	if err := cmd.Start(); err != nil {
		return 0, buf.Bytes(), fmt.Errorf("start command: %w", err)
	}
	job, jerr := newJobObject()
	if jerr != nil {
		_ = cmd.Process.Kill()
		return 0, buf.Bytes(), fmt.Errorf("create job object: %w", jerr)
	}
	if err := job.assign(uint32(cmd.Process.Pid)); err != nil {
		job.close()
		_ = cmd.Process.Kill()
		return 0, buf.Bytes(), fmt.Errorf("assign job object: %w", err)
	}
	// Resume the suspended process now that it is in the job.
	if p, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid)); err == nil {
		windows.ResumeThread(p)
		windows.CloseHandle(p)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		job.close()
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
		_ = job.terminate()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
		job.close()
		return -1, buf.Bytes(), ctx.Err()
	}
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
