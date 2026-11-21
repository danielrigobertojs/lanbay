// SPDX-License-Identifier: Apache-2.0
package software

import (
	"fmt"
	"strings"
)

// Aliases for callers/tests that use the winget-prefixed names.
var (
	ParseWingetSearch        = ParseSearch
	ParseWingetList          = ParseInstalledList
	ParseWingetUpgrade       = ParseUpgrade
	ParseSearchOutput        = ParseSearch
	ParseListOutput          = ParseInstalledList
	ParseUpgradeOutput       = ParseUpgrade
	ParseUpdates             = ParseUpgrade
	ParseWingetSearchOutput  = ParseSearch
	ParseWingetListOutput    = ParseInstalledList
	ParseWingetUpgradeOutput = ParseUpgrade
)

// ParseSearch parses the fixed-width text output of `winget search`.
func ParseSearch(output string) ([]SearchResult, error) {
	return parseWingetTable(output, map[string]string{
		"name":          "name",
		"nombre":        "name",
		"id":            "id",
		"identificador": "id",
		"version":       "version",
		"versión":       "version",
		"vers":          "version",
		"match":         "match",
		"coincidencia":  "match",
		"source":        "source",
		"origen":        "source",
		"publisher":     "publisher",
		"publicador":    "publisher",
	})
}

// ParseList parses `winget list` output.
func ParseList(output string) ([]ListResult, error) {
	return ParseInstalledList(output)
}

// ParseInstalledList parses `winget list` output into list results.
type ListResult struct {
	Name    string `json:"name"`
	ID      string `json:"id"`
	Version string `json:"version"`
	Source  string `json:"source,omitempty"`
}

func ParseInstalledList(output string) ([]ListResult, error) {
	cols, err := fixedColumns(output)
	if err != nil {
		// Recognise "no results" messages.
		lower := strings.ToLower(output)
		if strings.Contains(lower, "no installed") || strings.Contains(lower, "no se encontr") || strings.Contains(lower, "no package") {
			return nil, nil
		}
		return nil, err
	}
	// map headers
	header := cols.header
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, names ...string) string {
		for _, n := range names {
			if i, ok := idx[n]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}
	var result []ListResult
	for _, row := range cols.rows {
		r := ListResult{
			Name:    get(row, "name", "nombre"),
			ID:      get(row, "id", "identificador"),
			Version: get(row, "version", "versión", "vers"),
			Source:  get(row, "source", "origen"),
		}
		if r.ID == "" && r.Name == "" {
			continue
		}
		result = append(result, r)
	}
	if len(result) == 0 {
		lower := strings.ToLower(output)
		if !(strings.Contains(lower, "no installed") || strings.Contains(lower, "no se encontr") || strings.Contains(lower, "no package") || strings.Contains(lower, "no hay")) {
			// If there was a table but no data rows, that's an empty result, not an error.
		}
	}
	return result, nil
}

// ParseUpgrade parses `winget upgrade` output.
func ParseUpgrade(output string) ([]Update, error) {
	cols, err := fixedColumns(output)
	if err != nil {
		lower := strings.ToLower(output)
		if strings.Contains(lower, "no installed") || strings.Contains(lower, "no se encontr") || strings.Contains(lower, "no package") || strings.Contains(lower, "up to date") || strings.Contains(lower, "todo actualizado") || strings.Contains(lower, "no hay") {
			return nil, nil
		}
		return nil, err
	}
	header := cols.header
	idx := map[string]int{}
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	get := func(row []string, names ...string) string {
		for _, n := range names {
			if i, ok := idx[n]; ok && i < len(row) {
				return strings.TrimSpace(row[i])
			}
		}
		return ""
	}
	var result []Update
	for _, row := range cols.rows {
		u := Update{
			Name:             get(row, "name", "nombre"),
			ID:               get(row, "id", "identificador"),
			InstalledVersion: get(row, "version", "versión", "vers"),
			AvailableVersion: get(row, "available", "disponible"),
			Source:           get(row, "source", "origen"),
		}
		if u.ID == "" {
			continue
		}
		result = append(result, u)
	}
	return result, nil
}

// SearchResult parser using fixedColumns.
func parseWingetTable(output string, fieldMap map[string]string) ([]SearchResult, error) {
	cols, err := fixedColumns(output)
	if err != nil {
		lower := strings.ToLower(output)
		if strings.Contains(lower, "no package found") || strings.Contains(lower, "no se encontr") || strings.Contains(lower, "no packages") || strings.Contains(lower, "no hay") {
			return nil, nil
		}
		return nil, err
	}
	header := cols.header
	// map header -> field
	fieldIndex := map[string]int{} // field -> column index
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		if field, ok := fieldMap[key]; ok {
			fieldIndex[field] = i
		}
	}
	get := func(row []string, field string) string {
		if i, ok := fieldIndex[field]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var result []SearchResult
	for _, row := range cols.rows {
		r := SearchResult{
			Name:      get(row, "name"),
			ID:        get(row, "id"),
			Version:   get(row, "version"),
			Publisher: get(row, "publisher"),
			Match:     get(row, "match"),
			Source:    get(row, "source"),
		}
		if r.ID == "" && r.Name == "" {
			continue
		}
		result = append(result, r)
	}
	return result, nil
}

type fixedTable struct {
	header []string
	rows   [][]string
}

func fixedColumns(output string) (*fixedTable, error) {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	// find header + separator
	headerIdx := -1
	for i := 0; i+1 < len(lines); i++ {
		if isSeparator(lines[i+1]) && looksLikeHeader(lines[i]) {
			headerIdx = i
			break
		}
	}
	if headerIdx < 0 {
		return nil, fmt.Errorf("winget output not recognised: missing header")
	}
	headerLine := lines[headerIdx]
	sepLine := lines[headerIdx+1]
	starts := columnStarts(headerLine)
	if len(starts) == 0 {
		return nil, fmt.Errorf("winget output not recognised: no columns")
	}
	// parse header tokens
	header := splitByStarts(headerLine, starts)
	// parse data rows until blank or footer
	var rows [][]string
	for i := headerIdx + 2; i < len(lines); i++ {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			break
		}
		if isFooter(line) {
			break
		}
		// skip separator repeats
		if isSeparator(line) {
			continue
		}
		// ensure line has enough length; pad
		row := splitByStarts(line, starts)
		// skip if all empty
		allEmpty := true
		for _, c := range row {
			if strings.TrimSpace(c) != "" {
				allEmpty = false
				break
			}
		}
		if allEmpty {
			continue
		}
		rows = append(rows, row)
	}
	_ = sepLine
	return &fixedTable{header: header, rows: rows}, nil
}

func columnStarts(header string) []int {
	var starts []int
	in := false
	for i, r := range header {
		if r != ' ' && r != '\t' {
			if !in {
				starts = append(starts, i)
				in = true
			}
		} else {
			in = false
		}
	}
	return starts
}

func splitByStarts(line string, starts []int) []string {
	// pad line
	res := make([]string, len(starts))
	for i, s := range starts {
		end := len(line)
		if i+1 < len(starts) {
			end = starts[i+1]
		}
		if s >= len(line) {
			res[i] = ""
			continue
		}
		if end > len(line) {
			end = len(line)
		}
		res[i] = line[s:end]
	}
	return res
}

func isSeparator(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	dashes := 0
	for _, r := range t {
		if r == '-' || r == '—' || r == '=' {
			dashes++
		} else if r != ' ' {
			return false
		}
	}
	return dashes >= 3
}

func looksLikeHeader(line string) bool {
	lower := strings.ToLower(line)
	if !strings.Contains(lower, "id") {
		return false
	}
	return strings.Contains(lower, "name") || strings.Contains(lower, "nombre") ||
		strings.Contains(lower, "version") || strings.Contains(lower, "versión")
}

func isFooter(line string) bool {
	lower := strings.ToLower(strings.TrimSpace(line))
	return strings.HasPrefix(lower, "upgrades available") ||
		strings.HasPrefix(lower, "actualizaciones disponibles") ||
		strings.HasPrefix(lower, "packages found") ||
		strings.HasPrefix(lower, "paquetes encontrados") ||
		strings.Contains(lower, "upgrade available") ||
		strings.HasPrefix(lower, "the following packages")
}
