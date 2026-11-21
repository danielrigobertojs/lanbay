// SPDX-License-Identifier: Apache-2.0
package software

import (
	"strings"
	"testing"
)

func TestParseWingetSourceExport(t *testing.T) {
	out := `{"Arg":"https://cdn.winget.microsoft.com/cache","Data":"Microsoft.Winget.Source_8wekyb3d8bbwe","Explicit":false,"Identifier":"Microsoft.Winget.Source_8wekyb3d8bbwe","Name":"winget","TrustLevel":["Trusted","StoreOrigin"],"Type":"Microsoft.PreIndexed.Package"}`
	s, err := ParseWingetSourceExport(out)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "winget" || s.Arg != "https://cdn.winget.microsoft.com/cache" {
		t.Fatalf("parsed wrong: %+v", s)
	}
	// extra text around JSON
	s2, err := ParseWingetSourceExport("noise\n" + out + "\nmore")
	if err != nil || s2.Arg != s.Arg {
		t.Fatalf("extract failed: %v %+v", err, s2)
	}
	if _, err := ParseWingetSourceExport(""); err == nil {
		t.Fatal("expected error on empty")
	}
	if _, err := ParseWingetSourceExport("not json"); err == nil {
		t.Fatal("expected error on garbage")
	}
}

func TestVerifyWingetSourceExport(t *testing.T) {
	good := `{"Name":"winget","Arg":"https://cdn.winget.microsoft.com/cache"}`
	if err := VerifyWingetSourceExport(good); err != nil {
		t.Fatalf("good rejected: %v", err)
	}
	// hostile: redirected source
	bad := `{"Name":"winget","Arg":"https://evil.example.com/cache"}`
	if err := VerifyWingetSourceExport(bad); err == nil {
		t.Fatal("expected rejection of non-CDN source")
	}
	// hostile: different source name
	bad2 := `{"Name":"msstore","Arg":"https://cdn.winget.microsoft.com/cache"}`
	if err := VerifyWingetSourceExport(bad2); err == nil {
		t.Fatal("expected rejection of non-winget source name")
	}
	// hostile: trailing junk / injection is rejected
	if err := VerifyWingetSourceExport(`{"Name":"winget","Arg":"https://cdn.winget.microsoft.com/cache"}{"Name":"x","Arg":"y"}`); err == nil {
		t.Fatal("expected rejection of trailing junk")
	}
}

func TestVerifyWingetSettingsExport(t *testing.T) {
	// defaults: empty
	if err := VerifyWingetSettingsExport(""); err != nil {
		t.Fatalf("empty rejected: %v", err)
	}
	// safe settings
	safe := `{"visual":{"progressBar":"accent"},"source":{"autoUpdateIntervalInMinutes":15}}`
	if err := VerifyWingetSettingsExport(safe); err != nil {
		t.Fatalf("safe rejected: %v", err)
	}
	// hostile: InstallerHashOverride enabled
	bad := `{"security":{"InstallerHashOverride":true}}`
	if err := VerifyWingetSettingsExport(bad); err == nil {
		t.Fatal("expected rejection of InstallerHashOverride")
	}
	// hostile: LocalManifestFiles enabled (nested)
	bad2 := `{"experimentalFeatures":{"LocalManifestFiles":true}}`
	if err := VerifyWingetSettingsExport(bad2); err == nil {
		t.Fatal("expected rejection of LocalManifestFiles")
	}
	// disabled (false) is fine
	ok := `{"security":{"InstallerHashOverride":false,"LocalManifestFiles":false}}`
	if err := VerifyWingetSettingsExport(ok); err != nil {
		t.Fatalf("disabled rejected: %v", err)
	}
	// garbage
	if err := VerifyWingetSettingsExport("not json at all"); err == nil {
		t.Fatal("expected error on garbage")
	}
}

func TestValidateResultSource(t *testing.T) {
	cases := []struct {
		src   string
		valid bool
	}{
		{"", true},
		{"winget", true},
		{"WINGET", true},
		{"msstore", false},
		{"winget-font", false},
		{"https://evil", false},
		{"winget --source msstore", false},
	}
	for _, c := range cases {
		err := ValidateResultSource(c.src)
		if c.valid && err != nil {
			t.Errorf("expected valid %q: %v", c.src, err)
		}
		if !c.valid && err == nil {
			t.Errorf("expected invalid %q", c.src)
		}
	}
	// hostile: embedded control chars
	if err := ValidateResultSource("winget\x00msstore"); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestDecideInstalledRejectsBadSource(t *testing.T) {
	out := "Name  Id        Version  Source\n----  --        -------  ------\n7-Zip 7zip.7zip 24.00    msstore\n"
	ok, err := DecideInstalled(0, out, "7zip.7zip")
	if ok || err == nil {
		t.Fatalf("expected reject for non-winget source, got %v,%v", ok, err)
	}
	if !strings.Contains(err.Error(), "fuente no permitida") {
		t.Fatalf("unexpected err: %v", err)
	}
}
