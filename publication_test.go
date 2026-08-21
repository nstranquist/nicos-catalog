package catalog

import (
	"encoding/json"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"go.yaml.in/yaml/v3"
)

func TestPublicationVersionPinsAgree(t *testing.T) {
	version := strings.TrimSpace(readPublicationFile(t, "VERSION"))
	if !regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(version) {
		t.Fatalf("VERSION is not a stable SemVer tag: %q", version)
	}
	plain := strings.TrimPrefix(version, "v")
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(readPublicationFile(t, "explorer", "package.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version != plain {
		t.Fatalf("Explorer version = %q, want %q", manifest.Version, plain)
	}
	for _, path := range [][]string{{"README.md"}, {"site", "src", "content", "docs", "install.md"}, {"docs", "releases", version + ".md"}} {
		if content := readPublicationFile(t, path...); !strings.Contains(content, "@"+version) && !strings.Contains(content, " "+version) {
			t.Errorf("%s does not pin %s", filepath.Join(path...), version)
		}
	}
}

func TestPublicationReleaseNotesAreTimeless(t *testing.T) {
	version := strings.TrimSpace(readPublicationFile(t, "VERSION"))
	notes := readPublicationFile(t, "docs", "releases", version+".md")
	firstLine := publicationHeading(notes)
	if want := "# Nicos Catalog " + version; firstLine != want {
		t.Fatalf("release heading = %q, want %q", firstLine, want)
	}
	lower := strings.ToLower(notes)
	for _, transient := range []string{
		"release candidate",
		"status on ",
		"pending on this checkout",
		"operator publication gates",
		"operator must complete",
		"public tag and github release exist",
	} {
		if strings.Contains(lower, transient) {
			t.Errorf("release notes contain transient status text %q", transient)
		}
	}
}

func TestPublicationHeadingAcceptsPortableLineEndings(t *testing.T) {
	const want = "# Nicos Catalog v9.8.7"
	for name, notes := range map[string]string{
		"LF":   want + "\n\nNotes\n",
		"CRLF": want + "\r\n\r\nNotes\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := publicationHeading(notes); got != want {
				t.Fatalf("publicationHeading() = %q, want %q", got, want)
			}
		})
	}
}

func TestPortfolioManifestReferencesREADMEHeadings(t *testing.T) {
	var manifest any
	if err := yaml.Unmarshal([]byte(readPublicationFile(t, "portfolio", "manifest.yaml")), &manifest); err != nil {
		t.Fatal(err)
	}
	anchors := markdownHeadingAnchors(readPublicationFile(t, "README.md"))
	for _, value := range stringValues(manifest) {
		target, err := url.Parse(value)
		if err != nil || !strings.EqualFold(target.Host, "github.com") || target.Path != "/nstranquist/nicos-catalog" || target.Fragment == "" {
			continue
		}
		if !anchors[target.Fragment] {
			t.Errorf("portfolio URL fragment %q does not name a README heading", target.Fragment)
		}
	}
}

func TestPublicationHasNoLocalExplorerDependencies(t *testing.T) {
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(readPublicationFile(t, "explorer", "package.json")), &manifest); err != nil {
		t.Fatal(err)
	}
	for name, value := range mergeDependencies(manifest.Dependencies, manifest.DevDependencies) {
		lower := strings.ToLower(strings.TrimSpace(value))
		for _, prefix := range []string{"file:", "link:", "workspace:", "http:", "https:", "git:"} {
			if strings.HasPrefix(lower, prefix) {
				t.Errorf("dependency %s uses non-registry source %q", name, value)
			}
		}
	}
}

func TestPublicationExplorerSourceIsPortable(t *testing.T) {
	forbidden := []string{
		"nicos" + "-tools",
		string(filepath.Separator) + "Users" + string(filepath.Separator),
		"http" + "://",
		"https" + "://",
	}
	err := filepath.WalkDir(filepath.Join("explorer", "src"), func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, fragment := range forbidden {
			if strings.Contains(string(content), fragment) {
				t.Errorf("%s contains forbidden source fragment %q", path, fragment)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPublicationExplorerDistIsClosed(t *testing.T) {
	index := readPublicationFile(t, "explorer", "dist", "index.html")
	if strings.Contains(index, "http://") || strings.Contains(index, "https://") || !strings.Contains(index, "/assets/app-") {
		t.Fatalf("Explorer index does not use closed local assets")
	}
	entries, err := os.ReadDir(filepath.Join("explorer", "dist", "assets"))
	if err != nil {
		t.Fatal(err)
	}
	var app, styles, routeChunks int
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case strings.HasPrefix(name, "app-") && strings.HasSuffix(name, ".js"):
			app++
		case strings.HasPrefix(name, "index-") && strings.HasSuffix(name, ".css"):
			styles++
		case strings.HasPrefix(name, "route-") && strings.HasSuffix(name, ".js"):
			routeChunks++
		}
		if strings.HasSuffix(name, ".map") {
			t.Errorf("source map is present in Explorer dist: %s", name)
		}
	}
	if app != 1 || styles != 1 || routeChunks < 5 {
		t.Fatalf("Explorer assets = app:%d css:%d route chunks:%d", app, styles, routeChunks)
	}
}

func readPublicationFile(t *testing.T, parts ...string) string {
	t.Helper()
	payload, err := os.ReadFile(filepath.Join(parts...))
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func publicationHeading(notes string) string {
	firstLine, _, _ := strings.Cut(notes, "\n")
	return strings.TrimSuffix(firstLine, "\r")
}

func markdownHeadingAnchors(markdown string) map[string]bool {
	anchors := map[string]bool{}
	for _, line := range strings.Split(markdown, "\n") {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		heading := strings.TrimSpace(strings.TrimLeft(line, "#"))
		var anchor strings.Builder
		for _, char := range strings.ToLower(heading) {
			switch {
			case unicode.IsLetter(char), unicode.IsNumber(char), char == '-', char == '_':
				anchor.WriteRune(char)
			case unicode.IsSpace(char):
				anchor.WriteByte('-')
			}
		}
		if anchor.Len() > 0 {
			anchors[anchor.String()] = true
		}
	}
	return anchors
}

func stringValues(value any) []string {
	var values []string
	switch typed := value.(type) {
	case string:
		values = append(values, typed)
	case []any:
		for _, item := range typed {
			values = append(values, stringValues(item)...)
		}
	case map[string]any:
		for _, item := range typed {
			values = append(values, stringValues(item)...)
		}
	}
	return values
}

func mergeDependencies(groups ...map[string]string) map[string]string {
	merged := map[string]string{}
	for _, group := range groups {
		for name, value := range group {
			merged[name] = value
		}
	}
	return merged
}
