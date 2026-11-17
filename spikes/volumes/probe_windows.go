// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/json"
	"flag"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var kernel = syscall.NewLazyDLL("kernel32.dll")
var getDrives = kernel.NewProc("GetLogicalDrives")
var getType = kernel.NewProc("GetDriveTypeW")
var getInfo = kernel.NewProc("GetVolumeInformationW")
var queryDos = kernel.NewProc("QueryDosDeviceW")

type volume struct {
	Root           string `json:"root"`
	DriveType      uint32 `json:"drive_type"`
	Label          string `json:"label"`
	FileSystem     string `json:"file_system"`
	Serial         uint32 `json:"serial"`
	DosDevice      string `json:"dos_device"`
	CandidateLocal bool   `json:"candidate_local"`
}

func z(s []uint16) string {
	for i, c := range s {
		if c == 0 {
			return syscall.UTF16ToString(s[:i])
		}
	}
	return syscall.UTF16ToString(s)
}
func main() {
	out := flag.String("out", "volumes.json", "")
	reparse := flag.String("reparse-path", "", "")
	flag.Parse()
	mask, _, _ := getDrives.Call()
	r := map[string]any{"generated_at": time.Now().UTC(), "volumes": []volume{}, "reparse_path": *reparse}
	vs := []volume{}
	for i := uintptr(0); i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		p, _ := syscall.UTF16PtrFromString(root)
		typ, _, _ := getType.Call(uintptr(unsafe.Pointer(p)))
		label := make([]uint16, 261)
		fs := make([]uint16, 261)
		var serial, max, flags uint32
		getInfo.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&label[0])), uintptr(len(label)), uintptr(unsafe.Pointer(&serial)), uintptr(unsafe.Pointer(&max)), uintptr(unsafe.Pointer(&flags)), uintptr(unsafe.Pointer(&fs[0])), uintptr(len(fs)))
		name, _ := syscall.UTF16PtrFromString(root[:2])
		dos := make([]uint16, 32768)
		n, _, _ := queryDos.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&dos[0])), uintptr(len(dos)))
		v := volume{root, uint32(typ), z(label), z(fs), serial, z(dos[:n]), typ == 2 || typ == 3}
		vs = append(vs, v)
	}
	r["volumes"] = vs
	if *reparse != "" {
		r["reparse_path_exists"] = func() bool { _, e := os.Lstat(*reparse); return e == nil }()
	}
	b, _ := json.MarshalIndent(r, "", "  ")
	_ = os.MkdirAll("out", 0755)
	_ = os.WriteFile(*out, b, 0644)
}
