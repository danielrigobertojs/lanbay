// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type one struct {
	Root                    string `json:"root"`
	FileSystem              string `json:"file_system"`
	RecycleDirectoryPresent bool   `json:"recycle_directory_present"`
	PowerShellExit          int    `json:"powershell_exit"`
	Output                  string `json:"output"`
	FileGone                bool   `json:"file_gone"`
}

func main() {
	roots := flag.String("roots", "", "")
	script := flag.String("script", "recycle.ps1", "")
	out := flag.String("out", "recycle-bin.json", "")
	flag.Parse()
	rs := []one{}
	for _, root := range strings.Split(*roots, ";") {
		if root == "" {
			continue
		}
		f := filepath.Join(root, "lanbay-m0-recycle-"+time.Now().Format("20060102150405")+".tmp")
		x := one{Root: root}
		x.FileSystem = "unknown"
		_, e := os.Stat(filepath.Join(root, "$Recycle.Bin"))
		x.RecycleDirectoryPresent = e == nil
		_ = os.WriteFile(f, []byte("safe disposable M0 recycle probe\n"), 0600)
		c := exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", *script, "-Path", f)
		b, e := c.CombinedOutput()
		if e != nil {
			x.PowerShellExit = 1
			if z, ok := e.(*exec.ExitError); ok {
				x.PowerShellExit = z.ExitCode()
			}
		}
		x.Output = string(b)
		_, e = os.Stat(f)
		x.FileGone = os.IsNotExist(e)
		rs = append(rs, x)
	}
	v := map[string]any{"generated_at": time.Now().UTC(), "results": rs}
	b, _ := json.MarshalIndent(v, "", "  ")
	_ = os.WriteFile(*out, b, 0644)
}
