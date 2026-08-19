package hostcollate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeSettingsOffByDefault(t *testing.T) {
	settings, err := DecodeSettings(nil)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Active() {
		t.Fatal("empty payload must not be active")
	}
	if !settings.RegistrationRequired() {
		t.Fatal("require_registration must default true")
	}
}

func TestLoadSettingsMissingFileIsDisabled(t *testing.T) {
	settings, err := LoadSettings(filepath.Join(t.TempDir(), "settings.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if settings.GitHub.Collation.Enabled || settings.Active() {
		t.Fatalf("missing file must be disabled: %+v", settings)
	}
}

func TestDecodeSettingsEnabledNeedsProfile(t *testing.T) {
	settings, err := DecodeSettings([]byte(`
github:
  collation:
    enabled: true
    roots: ["./repos"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if settings.GitHub.Collation.Enabled != true {
		t.Fatal("enabled should parse")
	}
	if settings.Active() {
		t.Fatal("enabled without profile must contribute zero records")
	}
}

func TestDecodeSettingsActive(t *testing.T) {
	settings, err := DecodeSettings([]byte(`
github:
  profile: nstranquist
  collation:
    enabled: true
    roots:
      - ~/dev
      - /abs/tools
`))
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Active() {
		t.Fatal("profile + enabled must be active")
	}
	roots, err := settings.ExpandRoots("/Users/nico", "/host")
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 || filepath.ToSlash(roots[0]) != "/Users/nico/dev" || filepath.ToSlash(roots[1]) != "/abs/tools" {
		t.Fatalf("roots = %#v", roots)
	}
}

func TestDecodeSettingsRequireRegistrationFalseFailsClosed(t *testing.T) {
	_, err := DecodeSettings([]byte(`
github:
  profile: nstranquist
  collation:
    enabled: true
    require_registration: false
`))
	if err == nil {
		t.Fatal("require_registration false must fail closed")
	}
}

func TestDecodeSettingsUnknownFieldFailsClosed(t *testing.T) {
	_, err := DecodeSettings([]byte("github:\n  nickname: nico\n"))
	if err == nil {
		t.Fatal("unknown field must fail closed")
	}
}

func TestDecodeSettingsUnknownRefreshFailsClosed(t *testing.T) {
	_, err := DecodeSettings([]byte(`
github:
  profile: nstranquist
  collation:
    enabled: true
    refresh: implicit
`))
	if err == nil {
		t.Fatal("implicit refresh must fail closed")
	}
}

func TestLoadSettingsReadsRealFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, SettingsFileName)
	if err := os.WriteFile(path, []byte("github:\n  profile: nstranquist\n  collation:\n    enabled: true\n    roots: [repos]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Profile() != "nstranquist" || !settings.Active() {
		t.Fatalf("loaded %+v", settings)
	}
	roots, err := settings.ExpandRoots("", dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0] != filepath.Join(dir, "repos") {
		t.Fatalf("expanded roots = %#v", roots)
	}
}

func TestExpandRootsTildeWithoutHomeFailsClosed(t *testing.T) {
	settings, err := DecodeSettings([]byte(`
github:
  profile: nstranquist
  collation:
    enabled: true
    roots: ["~/dev"]
`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := settings.ExpandRoots("", "/host"); err == nil {
		t.Fatal("~/ root without home must fail closed")
	}
}

func TestDecodeSettingsTrailingDocumentFailsClosed(t *testing.T) {
	_, err := DecodeSettings([]byte("github:\n  profile: nstranquist\n---\nother: true\n"))
	if err == nil {
		t.Fatal("trailing YAML document must fail closed")
	}
}
