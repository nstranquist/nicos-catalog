package hostcollate

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// SettingsFileName is the host settings file inside Layout.ConfigDir.
const SettingsFileName = "settings.yaml"

// Settings is the host-owned collation configuration. The portable engine
// never reads this file.
type Settings struct {
	GitHub GitHubSettings `yaml:"github"`
}

// GitHubSettings names the opted-in GitHub identity and collation policy.
type GitHubSettings struct {
	Profile   string            `yaml:"profile"`
	Collation CollationSettings `yaml:"collation"`
}

// CollationSettings is the explicit opt-in for walking local clones.
type CollationSettings struct {
	Enabled             bool                     `yaml:"enabled"`
	Roots               []string                 `yaml:"roots"`
	RequireRegistration *bool                    `yaml:"require_registration"`
	Refresh             string                   `yaml:"refresh"`
	MaxRepos            int                      `yaml:"max_repos"`
	SkipDirNames        []string                 `yaml:"skip_dir_names"`
	Compare             CollationCompareSettings `yaml:"compare"`
}

// CollationCompareSettings opts into a missing-clone bucket from an injected
// profile-repo list. It never invents catalog entities.
type CollationCompareSettings struct {
	Enabled bool `yaml:"enabled"`
}

// DefaultSettings returns collation off, registration required, explicit refresh.
func DefaultSettings() Settings {
	required := true
	return Settings{
		GitHub: GitHubSettings{
			Collation: CollationSettings{
				RequireRegistration: &required,
				Refresh:             "explicit",
			},
		},
	}
}

// Active reports whether collation should contribute any records.
func (s Settings) Active() bool {
	return s.GitHub.Collation.Enabled && strings.TrimSpace(s.GitHub.Profile) != ""
}

// Profile returns the trimmed configured GitHub profile, if any.
func (s Settings) Profile() string {
	return strings.TrimSpace(s.GitHub.Profile)
}

// RegistrationRequired reports the fail-closed default: only registered
// clones emit records.
func (s Settings) RegistrationRequired() bool {
	if s.GitHub.Collation.RequireRegistration == nil {
		return true
	}
	return *s.GitHub.Collation.RequireRegistration
}

// LoadSettings decodes path. A missing file is a disabled default, not an error.
func LoadSettings(path string) (Settings, error) {
	//nolint:gosec // The host explicitly supplies its settings path.
	payload, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return Settings{}, fmt.Errorf("read settings %s: %w", path, err)
	}
	return DecodeSettings(payload)
}

// DecodeSettings strictly decodes host settings. Unknown fields fail closed.
func DecodeSettings(payload []byte) (Settings, error) {
	if len(strings.TrimSpace(string(payload))) == 0 {
		return DefaultSettings(), nil
	}
	var settings Settings
	decoder := yaml.NewDecoder(strings.NewReader(string(payload)))
	decoder.KnownFields(true)
	if err := decoder.Decode(&settings); err != nil {
		return Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Settings{}, fmt.Errorf("decode settings: multiple YAML documents are not allowed")
		}
		return Settings{}, fmt.Errorf("decode settings: %w", err)
	}
	if settings.GitHub.Collation.RequireRegistration == nil {
		required := true
		settings.GitHub.Collation.RequireRegistration = &required
	}
	if strings.TrimSpace(settings.GitHub.Collation.Refresh) == "" {
		settings.GitHub.Collation.Refresh = "explicit"
	}
	if settings.GitHub.Collation.SkipDirNames == nil {
		settings.GitHub.Collation.SkipDirNames = []string{"node_modules", ".build"}
	}
	if err := settings.Validate(); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

// Validate rejects contradictory or unknown policy values. require_registration
// false fails closed because unregistered remotes must never become entities.
func (s Settings) Validate() error {
	if s.GitHub.Collation.RequireRegistration != nil && !*s.GitHub.Collation.RequireRegistration {
		return fmt.Errorf("decode settings: require_registration false is not allowed; unregistered remotes must not become entities")
	}
	refresh := strings.TrimSpace(s.GitHub.Collation.Refresh)
	if refresh != "" && refresh != "explicit" {
		return fmt.Errorf("decode settings: refresh %q is not allowed; only explicit is supported", refresh)
	}
	if s.GitHub.Collation.MaxRepos < 0 {
		return fmt.Errorf("decode settings: max_repos must be >= 0")
	}
	return nil
}

// WalkPolicy returns the walk budget and skip names from settings.
func (s Settings) WalkPolicy() WalkOptions {
	return WalkOptions{
		MaxRepos:     s.GitHub.Collation.MaxRepos,
		SkipDirNames: append([]string(nil), s.GitHub.Collation.SkipDirNames...),
	}
}

// CompareEnabled reports whether missing-clone compare is opted in.
func (s Settings) CompareEnabled() bool {
	return s.GitHub.Collation.Compare.Enabled
}

// ExpandRoots resolves configured roots against home and the host root.
// home is injected so tests never rewrite HOME. A ~/ root with an empty
// home fails closed instead of disappearing from the walk.
func (s Settings) ExpandRoots(home, hostRoot string) ([]string, error) {
	var roots []string
	seen := map[string]struct{}{}
	for _, raw := range s.GitHub.Collation.Roots {
		trimmed := strings.TrimSpace(raw)
		if needsHome(trimmed) && strings.TrimSpace(home) == "" {
			return nil, fmt.Errorf("settings: root %q requires a home directory", trimmed)
		}
		expanded := expandPath(trimmed, home, hostRoot)
		if expanded == "" {
			continue
		}
		if _, ok := seen[expanded]; ok {
			continue
		}
		seen[expanded] = struct{}{}
		roots = append(roots, expanded)
	}
	return roots, nil
}

func needsHome(raw string) bool {
	return raw == "~" || strings.HasPrefix(raw, "~/")
}

func expandPath(raw, home, hostRoot string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if raw == "~" {
		if strings.TrimSpace(home) == "" {
			return ""
		}
		return filepath.Clean(home)
	}
	if strings.HasPrefix(raw, "~/") {
		if strings.TrimSpace(home) == "" {
			return ""
		}
		return filepath.Clean(filepath.Join(home, raw[2:]))
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") {
		return filepath.Clean(raw)
	}
	if strings.TrimSpace(hostRoot) == "" {
		return filepath.Clean(raw)
	}
	return filepath.Clean(filepath.Join(hostRoot, raw))
}

// SettingsPath joins configDir with the canonical settings file name.
func SettingsPath(configDir string) string {
	return filepath.Join(configDir, SettingsFileName)
}
