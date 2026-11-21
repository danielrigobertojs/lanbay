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
