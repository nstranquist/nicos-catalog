package hostcollate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGitHubRemote(t *testing.T) {
	cases := []struct {
		raw         string
		owner, repo string
		ok          bool
	}{
		{"https://github.com/nstranquist/docs-puller.git", "nstranquist", "docs-puller", true},
		{"https://github.com/nstranquist/docs-puller", "nstranquist", "docs-puller", true},
		{"https://www.github.com/nstranquist/openbook.git", "nstranquist", "openbook", true},
		{"git@github.com:nstranquist/nicos-catalog.git", "nstranquist", "nicos-catalog", true},
		{"ssh://git@github.com/nstranquist/jobkit.git", "nstranquist", "jobkit", true},
		{"git://github.com/nstranquist/agent-ops.git", "nstranquist", "agent-ops", true},
		{"https://gitlab.com/nstranquist/docs-puller.git", "", "", false},
		{"not-a-url", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		owner, repo, ok := ParseGitHubRemote(tc.raw)
		if ok != tc.ok || owner != tc.owner || repo != tc.repo {
			t.Fatalf("ParseGitHubRemote(%q) = %q %q %v, want %q %q %v", tc.raw, owner, repo, ok, tc.owner, tc.repo, tc.ok)
		}
	}
}

func TestRemoteMatchesProfileCaseInsensitive(t *testing.T) {
	owner, repo, match := RemoteMatchesProfile("https://github.com/NStranquist/Docs-Puller.git", "nstranquist")
	if !match || owner != "NStranquist" || repo != "Docs-Puller" {
		t.Fatalf("got %q %q %v", owner, repo, match)
	}
	_, _, match = RemoteMatchesProfile("https://github.com/other/docs-puller.git", "nstranquist")
	if match {
		t.Fatal("wrong owner must not match")
	}
}

func TestIncludePathRemotes(t *testing.T) {
	dir := t.TempDir()
	extra := filepath.Join(dir, "extra.config")
	if err := os.WriteFile(extra, []byte("[remote \"origin\"]\n\turl = https://github.com/nstranquist/included.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(dir, "config")
	if err := os.WriteFile(main, []byte("[include]\n\tpath = extra.config\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadGitConfig(main, dir, "", map[string]struct{}{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, repo, _, ok := MatchingRemote(loaded.remotes, "nstranquist")
	if !ok || repo != "included" {
		t.Fatalf("included remotes = %#v", loaded.remotes)
	}
}

func TestInsteadOfRewritesAliasRemote(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := `[remote "origin"]
	url = gh:nstranquist/aliased.git
[url "git@github.com:"]
	insteadOf = gh:
`
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	remotes, err := remotesForClone(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	owner, name, url, ok := MatchingRemote(remotes, "nstranquist")
	if !ok || owner != "nstranquist" || name != "aliased" {
		t.Fatalf("rewritten remotes = %#v", remotes)
	}
	if url != "git@github.com:nstranquist/aliased.git" {
		t.Fatalf("canonical url = %q", url)
	}
}

func TestUserGitConfigInsteadOf(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[url \"https://github.com/\"]\n\tinsteadOf = gh:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	writeGitConfig(t, repo, "gh:nstranquist/from-home.git")
	remotes, err := remotesForClone(repo, home)
	if err != nil {
		t.Fatal(err)
	}
	_, name, _, ok := MatchingRemote(remotes, "nstranquist")
	if !ok || name != "from-home" {
		t.Fatalf("home insteadOf remotes = %#v", remotes)
	}
}

func TestIncludeIfGitdir(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, "repo", ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(root, "extra.config")
	if err := os.WriteFile(extra, []byte("[remote \"origin\"]\n\turl = https://github.com/nstranquist/conditional.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(gitDir, "config")
	body := "[includeIf \"gitdir:" + gitDir + "\"]\n\tpath = " + extra + "\n"
	if err := os.WriteFile(main, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadGitConfig(main, gitDir, "", map[string]struct{}{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, name, _, ok := MatchingRemote(loaded.remotes, "nstranquist")
	if !ok || name != "conditional" {
		t.Fatalf("includeIf remotes = %#v", loaded.remotes)
	}
}

func TestExpandGitPathAndIncludeCycle(t *testing.T) {
	if expandGitPath("~/x", "/cfg", "/Users/nico") != "/Users/nico/x" {
		t.Fatal("tilde include")
	}
	if expandGitPath("", "/cfg", "/h") != "" {
		t.Fatal("empty include")
	}
	dir := t.TempDir()
	a := filepath.Join(dir, "a")
	b := filepath.Join(dir, "b")
	if err := os.WriteFile(a, []byte("[include]\n\tpath = b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("[include]\n\tpath = a\n[remote \"origin\"]\n\turl = https://github.com/nstranquist/cycle.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadGitConfig(a, dir, "", map[string]struct{}{}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, name, _, ok := MatchingRemote(loaded.remotes, "nstranquist")
	if !ok || name != "cycle" {
		t.Fatalf("cycle include remotes = %#v", loaded.remotes)
	}
}

func TestIncludeIfGitdirInsensitiveAndPrefix(t *testing.T) {
	if !includeIfMatches(includeIf{kind: "gitdir/i", pattern: "/TMP/Repo/.git"}, "/tmp/repo/.git", "") {
		t.Fatal("gitdir/i should match")
	}
	if !includeIfMatches(includeIf{kind: "gitdir", pattern: "/src/"}, "/src/app/.git", "") {
		t.Fatal("gitdir prefix should match")
	}
	if includeIfMatches(includeIf{kind: "gitdir", pattern: "/src/"}, "/other/.git", "") {
		t.Fatal("unrelated gitdir must not match")
	}
	if includeIfMatches(includeIf{kind: "onbranch", pattern: "main"}, "/tmp/.git", "") {
		t.Fatal("onbranch is not implemented")
	}
}

func TestParseGitDirLine(t *testing.T) {
	if parseGitDirLine("# comment\ngitdir: /abs/git\n") != "/abs/git" {
		t.Fatal("absolute gitdir")
	}
	if parseGitDirLine("GITDIR: ../x\n") != "../x" {
		t.Fatal("case fold gitdir")
	}
	if parseGitDirLine("not-a-gitfile\n") != "" {
		t.Fatal("missing gitdir")
	}
}

func TestReadGitConfigRemotes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config")
	body := `[core]
	repositoryformatversion = 0
[remote "origin"]
	url = git@github.com:nstranquist/docs-puller.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[remote "upstream"]
	url = https://github.com/other/docs-puller.git
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	remotes, err := ReadGitConfigRemotes(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(remotes) != 2 {
		t.Fatalf("remotes = %#v", remotes)
	}
	owner, repo, url, ok := MatchingRemote(remotes, "nstranquist")
	if !ok || owner != "nstranquist" || repo != "docs-puller" || url == "" {
		t.Fatalf("match = %q %q %q %v", owner, repo, url, ok)
	}
}
