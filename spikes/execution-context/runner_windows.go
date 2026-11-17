// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

type result struct {
	Command    string `json:"command"`
	ExitCode   int    `json:"exit_code"`
	DurationMS int64  `json:"duration_ms"`
	Output     string `json:"output"`
}
type report struct {
	GeneratedAt   time.Time `json:"generated_at"`
	OS            string    `json:"os"`
	User          string    `json:"user"`
	Elevation     result    `json:"elevation"`
	WingetSearch  result    `json:"winget_search"`
	WingetInstall result    `json:"winget_install"`
	MSI           result    `json:"msi"`
	Interactive   result    `json:"interactive_installer"`
	WriteC        result    `json:"write_c"`
	WriteMicroSD  result    `json:"write_micro_sd"`
}

func run(name string, args ...string) result {
	start := time.Now()
	c := exec.Command(name, args...)
	b, e := c.CombinedOutput()
	code := 0
	if e != nil {
		code = 1
		if x, ok := e.(*exec.ExitError); ok {
			code = x.ExitCode()
		}
	}
	return result{strings.Join(append([]string{name}, args...), " "), code, time.Since(start).Milliseconds(), string(b)}
}
func writeProbe(root string) result {
	start := time.Now()
	f := filepath.Join(root, "lanbay-m0-write-probe.tmp")
	e := os.WriteFile(f, []byte("Lanbay M0 write probe\n"), 0600)
	if e == nil {
		e = os.Remove(f)
	}
	out := "ok"
	code := 0
	if e != nil {
		out = e.Error()
		code = 1
	}
	return result{"write/remove " + f, code, time.Since(start).Milliseconds(), out}
}
func main() {
	out := flag.String("out", "execution-context.json", "")
	sd := flag.String("micro-sd", "", "")
	msi := flag.String("test-msi", "", "")
	interactive := flag.String("interactive-installer", "", "")
	id := flag.String("winget-id", "7zip.7zip", "")
	allow := flag.Bool("allow-install", false, "")
	flag.Parse()
	r := report{GeneratedAt: time.Now().UTC(), OS: runtime.GOOS, User: os.Getenv("USERNAME")}
	r.Elevation = run("whoami", "/groups")
	r.WingetSearch = run("winget", "search", "--id", *id, "--exact", "--source", "winget")
	if *allow {
		r.WingetInstall = run("winget", "install", "--id", *id, "--exact", "--source", "winget", "--silent", "--accept-package-agreements", "--accept-source-agreements", "--disable-interactivity")
	} else {
		r.WingetInstall = result{Command: "not run: pass -allow-install", ExitCode: -1}
	}
	if *msi != "" {
		r.MSI = run("msiexec.exe", "/i", *msi, "/qn", "/norestart", "/l*v", filepath.Join(filepath.Dir(*out), "msi.log"))
	} else {
		r.MSI = result{Command: "not run: -test-msi required", ExitCode: -1}
	}
	if *interactive != "" {
		r.Interactive = run("cmd.exe", "/c", "start", "\"Lanbay M0 interactive\"", *interactive)
	} else {
		r.Interactive = result{Command: "not run: -interactive-installer required", ExitCode: -1}
	}
	r.WriteC = writeProbe(`C:\`)
	if *sd != "" {
		r.WriteMicroSD = writeProbe(*sd)
	} else {
		r.WriteMicroSD = result{Command: "not run: -micro-sd required", ExitCode: -1}
	}
	_ = os.MkdirAll(filepath.Dir(*out), 0755)
	b, _ := json.MarshalIndent(r, "", "  ")
	_ = os.WriteFile(*out, b, 0644)
}
