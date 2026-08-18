package hostcollate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemotesForCloneReadsWorktreeCommonConfig(t *testing.T) {
	root := t.TempDir()
	main := filepath.Join(root, "main")
	worktree := filepath.Join(root, "wt")
	gitDir := filepath.Join(main, ".git")
	wtGit := filepath.Join(gitDir, "worktrees", "wt")
	for _, dir := range []string{main, worktree, wtGit, filepath.Join(worktree, ".nicos")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte("[remote \"origin\"]\n\turl = git@github.com:nstranquist/collate-docs.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtGit, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: "+wtGit+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".nicos", "product.yaml"), []byte("id: product.collate-docs\nname: Worktree\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	clone, err := inspectClone(worktree, "")
	if err != nil {
		t.Fatal(err)
	}
	owner, repo, _, ok := MatchingRemote(clone.Remotes, "nstranquist")
	if !ok || owner != "nstranquist" || repo != "collate-docs" {
		t.Fatalf("worktree remotes = %#v", clone.Remotes)
	}
	item := classifyClone("nstranquist", clone)
	if item.Bucket != BucketCollated {
		t.Fatalf("worktree bucket = %q, want collated", item.Bucket)
	}
}

func TestCloneIdentityFallbacks(t *testing.T) {
	tests := []struct {
		name  string
		clone Clone
		want  string
	}{
		{
			name:  "origin remote",
			clone: Clone{Remotes: []Remote{{Name: "origin", URL: "git@github.com:NStranquist/Atlas.git"}}},
			want:  "repo:nstranquist/atlas",
		},
		{
			name:  "non-origin remote",
			clone: Clone{Remotes: []Remote{{Name: "upstream", URL: "https://github.com/NStranquist/Explorer.git"}}},
			want:  "repo:nstranquist/explorer",
		},
		{
			name:  "missing checkout",
			clone: Clone{Path: filepath.Join("missing", "checkout")},
			want:  "path:" + filepath.Join("missing", "checkout"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cloneIdentity(tt.clone); got != tt.want {
				t.Fatalf("cloneIdentity() = %q, want %q", got, tt.want)
			}
		})
	}

	checkout := t.TempDir()
	gitDir := filepath.Join(checkout, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedGitDir, err := filepath.EvalSymlinks(gitDir)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cloneIdentity(Clone{Path: checkout}), "gitdir:"+resolvedGitDir; got != want {
		t.Fatalf("gitdir identity = %q, want %q", got, want)
	}
}

func TestRemotesForCloneReadsRelativeGitdir(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "checkout")
	hidden := filepath.Join(root, "actual.git")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hidden, "config"), []byte("[remote \"origin\"]\n\turl = https://github.com/nstranquist/relative.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: ../actual.git\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	remotes, err := remotesForClone(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	_, name, _, ok := MatchingRemote(remotes, "nstranquist")
	if !ok || name != "relative" {
		t.Fatalf("relative gitdir remotes = %#v", remotes)
	}
}

func TestCacheOnlyNicosCatalogIsNotRegistration(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nicos-catalog", "cache"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeGitConfig(t, root, "https://github.com/nstranquist/cache-only.git")
	clone, err := inspectClone(root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(clone.Registrations) != 0 {
		t.Fatalf("cache-only layout registered: %#v", clone.Registrations)
	}
	item := classifyClone("nstranquist", clone)
	if item.Bucket != BucketUnregistered {
		t.Fatalf("cache-only bucket = %q, want unregistered", item.Bucket)
	}
}

func TestWalkRootsFollowsSymlinkRoot(t *testing.T) {
	real := t.TempDir()
	writeGitConfig(t, filepath.Join(real, "repo"), "https://github.com/nstranquist/rootlink.git")
	link := filepath.Join(t.TempDir(), "alias-root")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{link}, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 1 {
		t.Fatalf("symlink root clones = %#v", clones)
	}
}

func TestWalkRootsFindsBareRepo(t *testing.T) {
	root := t.TempDir()
	bare := filepath.Join(root, "project.git")
	writeBareGit(t, bare, "https://github.com/nstranquist/bare-repo.git")
	if err := os.MkdirAll(filepath.Join(bare, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, ".nicos", "product.yaml"), []byte("id: product.bare\nname: Bare\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{root}, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 1 {
		t.Fatalf("clones = %#v", clones)
	}
	item := classifyClone("nstranquist", clones[0])
	if item.Bucket != BucketCollated || item.Repo != "nstranquist/bare-repo" {
		t.Fatalf("bare item = %+v", item)
	}
}

func TestWalkRootsFollowsGitDirSymlink(t *testing.T) {
	root := t.TempDir()
	realGit := filepath.Join(root, "real.git")
	writeBareGit(t, realGit, "https://github.com/nstranquist/symlinked.git")
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(filepath.Join(work, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realGit, filepath.Join(work, ".git")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, ".nicos", "product.yaml"), []byte("id: product.symlink\nname: Sym\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{root}, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var workClone Clone
	for _, clone := range clones {
		if clone.Path == work {
			workClone = clone
		}
	}
	if workClone.Path == "" {
		t.Fatalf("missing work clone in %#v", clones)
	}
	item := classifyClone("nstranquist", workClone)
	if item.Bucket != BucketCollated {
		t.Fatalf("symlink .git bucket = %q remotes=%#v", item.Bucket, workClone.Remotes)
	}
}

func TestResolveGitDirSymlinkToGitfile(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(root, "actual.git")
	writeBareGit(t, actual, "https://github.com/nstranquist/via-file.git")
	gitfile := filepath.Join(root, "gitfile")
	if err := os.WriteFile(gitfile, []byte("gitdir: "+actual+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitfile, filepath.Join(work, ".git")); err != nil {
		t.Fatal(err)
	}
	remotes, err := remotesForClone(work, "")
	if err != nil {
		t.Fatal(err)
	}
	_, name, _, ok := MatchingRemote(remotes, "nstranquist")
	if !ok || name != "via-file" {
		t.Fatalf("symlink-to-gitfile remotes = %#v", remotes)
	}
}

func TestPreferRegisteredCheckoutOverShorterUnregistered(t *testing.T) {
	root := t.TempDir()
	short := filepath.Join(root, "a")
	long := filepath.Join(root, "registered-copy")
	writeGitConfig(t, short, "https://github.com/nstranquist/same.git")
	writeGitConfig(t, long, "https://github.com/nstranquist/same.git")
	if err := os.MkdirAll(filepath.Join(long, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(long, ".nicos", "product.yaml"), []byte("id: product.same\nname: Same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{root}, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 1 || clones[0].Path != long {
		t.Fatalf("should keep registered checkout, got %#v", clones)
	}
}

func TestWalkRootsFollowsCheckoutSymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	writeGitConfig(t, real, "https://github.com/nstranquist/linked-checkout.git")
	if err := os.MkdirAll(filepath.Join(real, ".nicos"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, ".nicos", "product.yaml"), []byte("id: product.linked\nname: Linked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "alias")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	clones, err := WalkRoots([]string{root}, WalkOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(clones) != 1 {
		t.Fatalf("symlink + real checkout should collapse to one clone: %#v", clones)
	}
	item := classifyClone("nstranquist", clones[0])
	if item.Bucket != BucketCollated || item.Repo != "nstranquist/linked-checkout" {
		t.Fatalf("deduped checkout = %+v from %#v", item, clones)
	}
	_ = link
}

func writeBareGit(t *testing.T, dir, remote string) {
	t.Helper()
	for _, sub := range []string{"objects", "refs"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("[remote \"origin\"]\n\turl = "+remote+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLayoutWithCorpusStillRegisters(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".nicos-catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "catalog"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", "service.x.yaml"), []byte("id: service.x\nname: X\nkind: service\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	regs := detectRegistrations(root)
	if len(regs) != 1 || regs[0].Kind != RegistrationLayout {
		t.Fatalf("registrations = %#v", regs)
	}
}
