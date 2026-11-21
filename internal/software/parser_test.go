// SPDX-License-Identifier: Apache-2.0
package software

import (
	"os"
	"testing"
)

func TestParseSearchFixture(t *testing.T) {
	b, err := os.ReadFile("testdata/winget-search-en.txt")
	if err != nil {
		t.Fatal(err)
	}
	res, err := ParseSearch(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected results")
	}
	if res[0].ID != "7zip.7zip" {
		t.Fatalf("unexpected first id: %s", res[0].ID)
	}
}

func TestParseSearchNoResults(t *testing.T) {
	out := "No package found matching input criteria.\n"
	res, err := ParseSearch(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty, got %d", len(res))
	}
}

func TestParseSearchUnrecognized(t *testing.T) {
	out := "this is not winget output\n"
	_, err := ParseSearch(out)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseUpgradeBasic(t *testing.T) {
	b, err := os.ReadFile("testdata/upgrade.txt")
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	res, err := ParseUpgrade(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].ID != "7zip.7zip" {
		t.Fatalf("unexpected: %+v", res)
	}
	if res[0].InstalledVersion != "24.00" || res[0].AvailableVersion != "25.00" {
		t.Fatalf("versions: %+v", res[0])
	}
}

func TestParseInstalledListPreexisting(t *testing.T) {
	out := `Name  Id        Version  Source
----  --        -------  ------
7-Zip 7zip.7zip 24.00   winget
`
	res, err := ParseInstalledList(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1, got %d", len(res))
	}
}
