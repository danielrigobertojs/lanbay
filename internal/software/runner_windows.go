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
	"unsafe"

	"golang.org/x/sys/windows"
)

const createSuspended = 0x00000004

func runCommandImpl(ctx context.Context, path string, args []string, env []string, dir string, out io.Writer) (int, []byte, error) {
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
		_ = cmd.Wait()
		return 0, buf.Bytes(), fmt.Errorf("create job object: %w", jerr)
	}
	if err := job.assign(uint32(cmd.Process.Pid)); err != nil {
		job.close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 0, buf.Bytes(), fmt.Errorf("assign job object: %w", err)
	}
	// Resume the suspended primary thread(s) now that the process is in the job.
	resumed := resumeProcessThreads(uint32(cmd.Process.Pid))
	if !resumed {
		job.close()
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return 0, buf.Bytes(), fmt.Errorf("no se pudo reanudar el proceso")
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

func resumeProcessThreads(pid uint32) bool {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(snapshot)
	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	if err := windows.Thread32First(snapshot, &entry); err != nil {
		return false
	}
	resumed := false
	for {
		if entry.OwnerProcessID == pid {
			if h, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID); err == nil {
				if n, err := windows.ResumeThread(h); err == nil && n != uint32(0xFFFFFFFF) {
					resumed = true
				}
				windows.CloseHandle(h)
			}
		}
		if err := windows.Thread32Next(snapshot, &entry); err != nil {
			break
		}
	}
	return resumed
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
