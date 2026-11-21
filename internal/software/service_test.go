// SPDX-License-Identifier: Apache-2.0
package software

import (
	"testing"
)

func TestValidateWingetID(t *testing.T) {
	cases := []struct {
		id    string
		valid bool
	}{
		{"7zip.7zip", true},
		{"Microsoft.VisualStudioCode", true},
		{"7zip --override x", false},
		{"--ignore-security-hash", false},
		{"a&calc", false},
		{"a\" --source msstore", false},
		{"-h", false},
		{"", false},
	}
	for _, c := range cases {
		err := validateWingetID(c.id)
		if c.valid && err != nil {
			t.Errorf("expected valid %q: %v", c.id, err)
		}
		if !c.valid && err == nil {
			t.Errorf("expected invalid %q", c.id)
		}
	}
}

func TestDecideInstalled(t *testing.T) {
	// not installed code
	ok, err := DecideInstalled(0x8A150014, "", "7zip.7zip")
	if ok || err != nil {
		t.Fatalf("expected false,nil got %v,%v", ok, err)
	}
	// code 0 exact match
	out := "Name  Id        Version  Source\n----  --        -------  ------\n7-Zip 7zip.7zip 24.00    winget\n"
	ok, err = DecideInstalled(0, out, "7zip.7zip")
	if !ok || err != nil {
		t.Fatalf("expected true,nil got %v,%v", ok, err)
	}
	// empty table with code 0 -> reject
	ok, err = DecideInstalled(0, "Name  Id  Version  Source\n----  --  -------  ------\n", "7zip.7zip")
	if ok || err == nil {
		t.Fatalf("expected reject got %v,%v", ok, err)
	}
	// unknown code -> reject
	ok, err = DecideInstalled(1, out, "7zip.7zip")
	if ok || err == nil {
		t.Fatalf("expected reject got %v,%v", ok, err)
	}
	// truncated id -> reject
	ok, err = DecideInstalled(0, out, "7zip.7zi")
	if ok || err == nil {
		t.Fatalf("expected reject got %v,%v", ok, err)
	}
	// unrecognized output -> reject
	ok, err = DecideInstalled(0, "this is not winget output", "7zip.7zip")
	if ok || err == nil {
		t.Fatalf("expected reject got %v,%v", ok, err)
	}
}

func TestValidateSearchQuery(t *testing.T) {
	if err := validateSearchQuery("7zip"); err != nil {
		t.Fatal(err)
	}
	if err := validateSearchQuery(string(make([]byte, 129))); err == nil {
		t.Fatal("expected too long")
	}
	if err := validateSearchQuery("-foo"); err == nil {
		t.Fatal("expected leading dash rejected")
	}
}
