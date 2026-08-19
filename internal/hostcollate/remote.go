package hostcollate

import (
	"bufio"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GitHubHost is the only remote host that participates in profile matching.
const GitHubHost = "github.com"

const maxGitConfigIncludes = 8

// Remote is one git remote URL plus its config name.
type Remote struct {
	Name string
	URL  string
}

type urlRewrite struct {
	prefix string
	with   string
}

type gitConfig struct {
	remotes   []Remote
	includes  []string
	includeIf []includeIf
	rewrites  []urlRewrite
}

type includeIf struct {
	kind    string
	pattern string
	path    string
}

// ParseGitHubRemote extracts owner and repo from a GitHub remote URL.
// HTTPS, SSH, git@, and git:// forms are accepted. Non-GitHub hosts fail.
func ParseGitHubRemote(raw string) (owner, repo string, ok bool) {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	if raw == "" {
		return "", "", false
	}
	if strings.HasPrefix(raw, "git@") {
		rest := strings.TrimPrefix(raw, "git@")
		host, path, found := strings.Cut(rest, ":")
		if !found {
			return "", "", false
		}
		return githubOwnerRepo(host, path)
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Path == "" {
		return "", "", false
	}
	return githubOwnerRepo(parsed.Host, parsed.Path)
}

// RemoteMatchesProfile reports whether raw is a GitHub remote owned by profile.
func RemoteMatchesProfile(raw, profile string) (owner, repo string, match bool) {
	owner, repo, ok := ParseGitHubRemote(raw)
	if !ok {
		return "", "", false
	}
	profile = strings.TrimSpace(profile)
	if profile == "" {
		return owner, repo, false
	}
	return owner, repo, strings.EqualFold(owner, profile)
}

func githubOwnerRepo(host, path string) (owner, repo string, ok bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "www.")
	if host != GitHubHost {
		return "", "", false
	}
	path = strings.TrimPrefix(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	owner = strings.TrimSpace(parts[0])
	repo = strings.TrimSpace(parts[1])
	if owner == "" || repo == "" {
		return "", "", false
	}
	return owner, repo, true
}

// ReadGitConfigRemotes parses remotes from a git config file without shelling
// out to git. Include files and insteadOf rewrites are not applied here; use
// remotesForClone for identity.
func ReadGitConfigRemotes(path string) ([]Remote, error) {
	parsed, err := parseGitConfigFile(path)
	if err != nil {
		return nil, err
	}
	return parsed.remotes, nil
}

func parseGitConfigFile(path string) (gitConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return gitConfig{}, err
	}
	defer func() { _ = file.Close() }()
	var parsed gitConfig
	var section, subsection string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section, subsection = parseGitSection(line[1 : len(line)-1])
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch {
		case section == "remote" && key == "url" && subsection != "":
			parsed.remotes = append(parsed.remotes, Remote{Name: subsection, URL: value})
		case section == "include" && key == "path" && subsection == "":
			parsed.includes = append(parsed.includes, value)
		case section == "includeif" && key == "path" && subsection != "":
			kind, pattern := parseIncludeIfCondition(subsection)
			if kind != "" {
				parsed.includeIf = append(parsed.includeIf, includeIf{kind: kind, pattern: pattern, path: value})
			}
		case section == "url" && key == "insteadof" && subsection != "":
			parsed.rewrites = append(parsed.rewrites, urlRewrite{prefix: value, with: subsection})
		}
	}
	if err := scanner.Err(); err != nil {
		return gitConfig{}, err
	}
	return parsed, nil
}

func parseGitSection(raw string) (section, subsection string) {
	raw = strings.TrimSpace(raw)
	kind, rest, ok := strings.Cut(raw, " ")
	section = strings.ToLower(strings.TrimSpace(kind))
	if !ok {
		return section, ""
	}
	return section, strings.Trim(strings.TrimSpace(rest), `"'`)
}

func parseIncludeIfCondition(raw string) (kind, pattern string) {
	raw = strings.TrimSpace(raw)
	kind, pattern, ok := strings.Cut(raw, ":")
	if !ok {
		return "", ""
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "gitdir", "gitdir/i":
		return kind, strings.TrimSpace(pattern)
	default:
		return "", ""
	}
}

func loadGitConfig(path, gitDir, home string, seen map[string]struct{}, depth int) (gitConfig, error) {
	var out gitConfig
	if depth > maxGitConfigIncludes {
		return out, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = filepath.Clean(path)
	}
	if _, dup := seen[abs]; dup {
		return out, nil
	}
	seen[abs] = struct{}{}
	parsed, err := parseGitConfigFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, fmt.Errorf("read git config %s: %w", path, err)
	}
	out.remotes = append(out.remotes, parsed.remotes...)
	out.rewrites = append(out.rewrites, parsed.rewrites...)
	configDir := filepath.Dir(path)
	for _, inc := range parsed.includes {
		resolved := expandGitPath(inc, configDir, home)
		if resolved == "" {
			continue
		}
		nested, nestErr := loadGitConfig(resolved, gitDir, home, seen, depth+1)
		if nestErr != nil {
			return out, nestErr
		}
		out.remotes = append(out.remotes, nested.remotes...)
		out.rewrites = append(out.rewrites, nested.rewrites...)
	}
	for _, cond := range parsed.includeIf {
		if !includeIfMatches(cond, gitDir, home) {
			continue
		}
		resolved := expandGitPath(cond.path, configDir, home)
		if resolved == "" {
			continue
		}
		nested, nestErr := loadGitConfig(resolved, gitDir, home, seen, depth+1)
		if nestErr != nil {
			return out, nestErr
		}
		out.remotes = append(out.remotes, nested.remotes...)
		out.rewrites = append(out.rewrites, nested.rewrites...)
	}
	return out, nil
}

func expandGitPath(raw, configDir, home string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if raw == "~" {
		return strings.TrimSpace(home)
	}
	if strings.HasPrefix(raw, "~/") && strings.TrimSpace(home) != "" {
		return filepath.Clean(filepath.Join(home, raw[2:]))
	}
	if filepath.IsAbs(raw) || strings.HasPrefix(raw, "/") {
		return filepath.Clean(raw)
	}
	return filepath.Clean(filepath.Join(configDir, raw))
}

func includeIfMatches(cond includeIf, gitDir, home string) bool {
	if cond.kind != "gitdir" && cond.kind != "gitdir/i" {
		return false
	}
	if gitDir == "" {
		return false
	}
	pattern := expandGitPath(cond.pattern, "", home)
	if pattern == "" {
		return false
	}
	dir := filepath.Clean(gitDir)
	fold := cond.kind == "gitdir/i"
	if fold {
		dir = strings.ToLower(dir)
		pattern = strings.ToLower(pattern)
	}
	prefix := strings.TrimRight(pattern, `/\`)
	if prefix == "" {
		return false
	}
	if dir == prefix {
		return true
	}
	sep := string(os.PathSeparator)
	return strings.HasPrefix(dir, prefix+sep)
}

func userGitConfigPaths(home string) []string {
	home = strings.TrimSpace(home)
	if home == "" {
		return nil
	}
	return []string{
		filepath.Join(home, ".gitconfig"),
		filepath.Join(home, ".config", "git", "config"),
	}
}

func loadUserRewrites(home string) []urlRewrite {
	var rewrites []urlRewrite
	seen := map[string]struct{}{}
	for _, path := range userGitConfigPaths(home) {
		parsed, err := loadGitConfig(path, "", home, seen, 0)
		if err != nil {
			continue
		}
		rewrites = append(rewrites, parsed.rewrites...)
	}
	return rewrites
}

func applyRewrites(remotes []Remote, rewrites []urlRewrite) []Remote {
	if len(rewrites) == 0 {
		return remotes
	}
	sorted := append([]urlRewrite(nil), rewrites...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return len(sorted[i].prefix) > len(sorted[j].prefix)
	})
	out := make([]Remote, len(remotes))
	for i, remote := range remotes {
		out[i] = canonicalizeRemote(remote, sorted)
	}
	return out
}

func canonicalizeRemote(remote Remote, rewrites []urlRewrite) Remote {
	if _, _, ok := ParseGitHubRemote(remote.URL); ok {
		return remote
	}
	for _, rw := range rewrites {
		if rw.prefix == "" || !strings.HasPrefix(remote.URL, rw.prefix) {
			continue
		}
		candidate := rw.with + strings.TrimPrefix(remote.URL, rw.prefix)
		if _, _, ok := ParseGitHubRemote(candidate); ok {
			remote.URL = candidate
			return remote
		}
	}
	return remote
}

// MatchingRemote returns the first remote that belongs to profile.
func MatchingRemote(remotes []Remote, profile string) (owner, repo, url string, ok bool) {
	for _, remote := range remotes {
		owner, repo, match := RemoteMatchesProfile(remote.URL, profile)
		if match {
			return owner, repo, remote.URL, true
		}
	}
	return "", "", "", false
}

// FirstGitHubRepo returns the first parseable GitHub remote, regardless of owner.
func FirstGitHubRepo(remotes []Remote) (owner, repo, url string, ok bool) {
	for _, remote := range remotes {
		owner, repo, parsed := ParseGitHubRemote(remote.URL)
		if parsed {
			return owner, repo, remote.URL, true
		}
	}
	return "", "", "", false
}
