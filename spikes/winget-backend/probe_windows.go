// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"flag"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type sample struct {
	Milliseconds int64  `json:"milliseconds"`
	ExitCode     int    `json:"exit_code"`
	Output       string `json:"output"`
}
type report struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Query       string         `json:"query"`
	Locale      string         `json:"locale"`
	CLI         []sample       `json:"cli_search"`
	WinRT       map[string]any `json:"winrt_activation"`
}

func cli(q string) sample {
	s := time.Now()
	c := exec.Command("winget", "search", "--query", q, "--source", "winget")
	b, e := c.CombinedOutput()
	code := 0
	if e != nil {
		code = 1
		if x, ok := e.(*exec.ExitError); ok {
			code = x.ExitCode()
		}
	}
	return sample{time.Since(s).Milliseconds(), code, string(b)}
}
func main() {
	out := flag.String("out", "winget-backend.json", "")
	q := flag.String("query", "7zip", "")
	n := flag.Int("iterations", 3, "")
	flag.Parse()
	r := report{GeneratedAt: time.Now().UTC(), Query: *q, Locale: os.Getenv("LANG"), WinRT: map[string]any{}}
	for i := 0; i < *n; i++ {
		r.CLI = append(r.CLI, cli(*q))
	} // RoActivateInstance confirms that the WinGet WinRT class is registered; catalog ABI calls require generated bindings.
	combase := syscall.NewLazyDLL("combase.dll")
	roInit := combase.NewProc("RoInitialize")
	roAct := combase.NewProc("RoActivateInstance")
	winstr := syscall.NewLazyDLL("combase.dll").NewProc("WindowsCreateString")
	del := combase.NewProc("WindowsDeleteString")
	name, _ := syscall.UTF16FromString("Microsoft.Management.Deployment.PackageManager")
	var hs uintptr
	hr, _, _ := winstr.Call(uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)-1), uintptr(unsafe.Pointer(&hs)))
	r.WinRT["create_hstring_hresult"] = int32(hr)
	if int32(hr) >= 0 {
		defer del.Call(hs)
		init, _, _ := roInit.Call(1)
		r.WinRT["ro_initialize_hresult"] = int32(init)
		var obj uintptr
		act, _, _ := roAct.Call(hs, uintptr(unsafe.Pointer(&obj)))
		r.WinRT["package_manager_activate_hresult"] = int32(act)
		r.WinRT["package_manager_pointer_nonzero"] = obj != 0
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	_ = os.WriteFile(*out, []byte(strings.TrimSpace(string(b))+"\n"), 0644)
}
