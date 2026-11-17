// SPDX-License-Identifier: Apache-2.0
// Command kitpack creates the M0 kit zip without relying on host zip utilities.
package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	source := flag.String("source", "", "directory to archive")
	output := flag.String("output", "", "zip output path")
	checksum := flag.String("sha256", "", "SHA-256 output path")
	flag.Parse()
	if *source == "" || *output == "" || *checksum == "" {
		fmt.Fprintln(os.Stderr, "-source, -output, and -sha256 are required")
		os.Exit(2)
	}
	if err := pack(*source, *output); err != nil {
		fmt.Fprintf(os.Stderr, "pack kit: %v\n", err)
		os.Exit(1)
	}
	if err := writeSHA256(*output, *checksum); err != nil {
		fmt.Fprintf(os.Stderr, "write checksum: %v\n", err)
		os.Exit(1)
	}
}

func pack(source, output string) error {
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	zw := zip.NewWriter(file)
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(rel)
		header.Method = zip.Deflate
		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		input, err := os.Open(path)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(writer, input)
		closeErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		return closeErr
	})
	if err != nil {
		_ = zw.Close()
		_ = file.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeSHA256(input, output string) error {
	f, err := os.Open(input)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	line := hex.EncodeToString(h.Sum(nil)) + "  " + filepath.Base(input) + "\n"
	return os.WriteFile(output, []byte(strings.TrimSpace(line)+"\n"), 0644)
}
