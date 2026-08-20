package hostcollate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Bucket is a report classification for one local clone.
type Bucket string

// Report buckets. Unregistered and wrong-owner clones never emit records.
const (
	BucketCollated     Bucket = "collated"
	BucketUnregistered Bucket = "unregistered"
	BucketWrongOwner   Bucket = "wrong_owner"
	BucketMissing      Bucket = "missing"
	BucketRejected     Bucket = "rejected"
	BucketDuplicate    Bucket = "duplicate"
	BucketGap          Bucket = "gap"
)

// RegistrationKind names the consent artifact found in a clone.
type RegistrationKind string

// Registration kinds the scanner accepts as catalog consent.
const (
	RegistrationNone        RegistrationKind = ""
	RegistrationProductYAML RegistrationKind = "product.yaml"
	RegistrationCorpus      RegistrationKind = "corpus"
	RegistrationLayout      RegistrationKind = "layout"
)

// Clone is one discovered git checkout plus the facts used to classify it.
type Clone struct {
	Path          string
	Remotes       []Remote
	Registrations []Registration
}

// Registration is one consent artifact inside a clone.
type Registration struct {
	Kind RegistrationKind
	Path string
}

// Item is one classified clone in the bucketed report.
type Item struct {
	Path      string   `json:"path"`
	Repo      string   `json:"repo,omitempty"`
	Remote    string   `json:"remote,omitempty"`
	Bucket    Bucket   `json:"bucket"`
	Source    string   `json:"source,omitempty"`
	EntityIDs []string `json:"entity_ids,omitempty"`
}

// WalkOptions bounds a local walk. MaxRepos 0 means unlimited.
type WalkOptions struct {
	Home         string
	MaxRepos     int
	SkipDirNames []string
	// Capped is set when the walk stops at MaxRepos, before identity collapse.
	Capped *bool
}

func (o WalkOptions) setCapped() {
	if o.Capped != nil {
		*o.Capped = true
	}
}

// WalkRoots finds git checkouts under roots. Configured root symlinks are
// followed. Child directory symlinks are treated as a checkout when they
// point at a git work tree or git dir, and are not descended into otherwise.
// home is used for ~ in git include paths and user insteadOf config.
func WalkRoots(roots []string, opts WalkOptions) ([]Clone, error) {
	skip := map[string]struct{}{}
	for _, name := range opts.SkipDirNames {
		name = strings.TrimSpace(name)
		if name != "" {
			skip[name] = struct{}{}
		}
	}
	var clones []Clone
	seen := map[string]struct{}{}
	add := func(path string) error {
		if opts.MaxRepos > 0 && len(clones) >= opts.MaxRepos {
			return filepath.SkipAll
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return nil
		}
		seen[clean] = struct{}{}
		clone, err := inspectClone(clean, opts.Home)
		if err != nil {
			return err
		}
		clones = append(clones, clone)
		if opts.MaxRepos > 0 && len(clones) >= opts.MaxRepos {
			return filepath.SkipAll
		}
		return nil
	}
	for _, root := range roots {
		info, err := os.Lstat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("stat root %s: %w", root, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, evalErr := filepath.EvalSymlinks(root)
			if evalErr != nil {
				continue
			}
			root = resolved
			info, err = os.Stat(root)
			if err != nil {
				continue
			}
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if _, skipped := skip[entry.Name()]; skipped {
				if entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.Name() == ".git" && entry.IsDir() {
				return filepath.SkipDir
			}
			if entry.Type()&os.ModeSymlink != 0 {
				if !isGitCheckout(path) {
					return nil
				}
				if err := add(path); err != nil {
					return err
				}
				return nil
			}
			if !entry.IsDir() {
				return nil
			}
			if !isGitCheckout(path) {
				return nil
			}
			if err := add(path); err != nil {
				return err
			}
			return filepath.SkipDir
		})
		if err != nil {
			return nil, fmt.Errorf("walk root %s: %w", root, err)
		}
	}
	sort.Slice(clones, func(i, j int) bool { return clones[i].Path < clones[j].Path })
	if opts.MaxRepos > 0 && len(clones) >= opts.MaxRepos {
		opts.setCapped()
	}
	return dedupeClones(clones), nil
}

// dedupeClones keeps one checkout per git identity (owner/repo or resolved
// gitdir). Two worktrees of nstranquist/blender-workspace are one clone.
func dedupeClones(clones []Clone) []Clone {
	best := map[string]Clone{}
	order := make([]string, 0, len(clones))
	for _, clone := range clones {
		key := cloneIdentity(clone)
		prev, ok := best[key]
		if !ok {
			best[key] = clone
			order = append(order, key)
			continue
		}
		if preferClone(clone, prev) {
			best[key] = clone
		}
	}
	out := make([]Clone, 0, len(order))
	for _, key := range order {
		out = append(out, best[key])
	}
	return out
}

func cloneIdentity(clone Clone) string {
	if owner, repo, ok := originGitHubRepo(clone.Remotes); ok {
		return "repo:" + strings.ToLower(owner+"/"+repo)
	}
	owner, repo, _, ok := FirstGitHubRepo(clone.Remotes)
	if ok {
		return "repo:" + strings.ToLower(owner+"/"+repo)
	}
	gitDir, err := resolveGitDir(clone.Path)
	if err == nil && gitDir != "" {
		if resolved, evalErr := filepath.EvalSymlinks(gitDir); evalErr == nil {
			gitDir = resolved
		}
		return "gitdir:" + filepath.Clean(gitDir)
	}
	return "path:" + filepath.Clean(clone.Path)
}

func preferClone(candidate, current Clone) bool {
	candReg := len(candidate.Registrations) > 0
	curReg := len(current.Registrations) > 0
	if candReg != curReg {
		return candReg
	}
	if len(candidate.Path) != len(current.Path) {
		return len(candidate.Path) < len(current.Path)
	}
	return candidate.Path < current.Path
}

func originGitHubRepo(remotes []Remote) (owner, repo string, ok bool) {
	for _, remote := range remotes {
		if !strings.EqualFold(remote.Name, "origin") {
			continue
		}
		owner, repo, parsed := ParseGitHubRemote(remote.URL)
		if parsed {
			return owner, repo, true
		}
	}
	return "", "", false
}

func isGitCheckout(path string) bool {
	return hasGitMeta(path) || isGitDir(path)
}

func hasGitMeta(path string) bool {
	info, err := os.Lstat(filepath.Join(path, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir() || info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0
}

func isGitDir(path string) bool {
	if !fileExists(filepath.Join(path, "HEAD")) {
		return false
	}
	if dirExists(filepath.Join(path, "objects")) && (dirExists(filepath.Join(path, "refs")) || fileExists(filepath.Join(path, "packed-refs"))) {
		return true
	}
	return fileExists(filepath.Join(path, "commondir"))
}

func inspectClone(path, home string) (Clone, error) {
	clone := Clone{Path: path, Registrations: detectRegistrations(path)}
	remotes, err := remotesForClone(path, home)
	if err != nil {
		return clone, err
	}
	clone.Remotes = remotes
	return clone, nil
}

func remotesForClone(clonePath, home string) ([]Remote, error) {
	gitDir, err := resolveGitDir(clonePath)
	if err != nil || gitDir == "" {
		return nil, err
	}
	loaded, err := loadGitConfig(filepath.Join(gitDir, "config"), gitDir, home, map[string]struct{}{}, 0)
	if err != nil {
		return nil, err
	}
	if len(loaded.remotes) == 0 {
		//nolint:gosec // resolveGitDir supplies the Git metadata directory.
		common, readErr := os.ReadFile(filepath.Join(gitDir, "commondir"))
		if readErr != nil && !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read commondir %s: %w", gitDir, readErr)
		}
		if readErr == nil {
			commonPath := strings.TrimSpace(string(common))
			if commonPath != "" {
				if !filepath.IsAbs(commonPath) {
					commonPath = filepath.Join(gitDir, commonPath)
				}
				commonPath = filepath.Clean(commonPath)
				nested, nestErr := loadGitConfig(filepath.Join(commonPath, "config"), commonPath, home, map[string]struct{}{}, 0)
				if nestErr != nil {
					return nil, nestErr
				}
				loaded.remotes = append(loaded.remotes, nested.remotes...)
				loaded.rewrites = append(loaded.rewrites, nested.rewrites...)
			}
		}
	}
	loaded.rewrites = append(loaded.rewrites, loadUserRewrites(home)...)
	return applyRewrites(loaded.remotes, loaded.rewrites), nil
}

func resolveGitDir(clonePath string) (string, error) {
	gitPath := filepath.Join(clonePath, ".git")
	info, err := os.Lstat(gitPath)
	if err != nil {
		if os.IsNotExist(err) && isGitDir(clonePath) {
			return clonePath, nil
		}
		//nolint:nilerr // An unreadable or invalid .git marker is not a usable checkout.
		return "", nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, evalErr := filepath.EvalSymlinks(gitPath)
		if evalErr != nil {
			//nolint:nilerr // A broken .git symlink is not a usable checkout.
			return "", nil
		}
		targetInfo, statErr := os.Lstat(target)
		if statErr != nil {
			//nolint:nilerr // A vanished .git symlink target is not a usable checkout.
			return "", nil
		}
		if targetInfo.IsDir() {
			return target, nil
		}
		if targetInfo.Mode().IsRegular() {
			return gitDirFromFile(target, filepath.Dir(target))
		}
		return "", nil
	}
	if info.IsDir() {
		return gitPath, nil
	}
	if info.Mode().IsRegular() {
		return gitDirFromFile(gitPath, clonePath)
	}
	return "", nil
}

func gitDirFromFile(gitFile, relativeTo string) (string, error) {
	//nolint:gosec // resolveGitDir supplies the discovered .git file path.
	payload, err := os.ReadFile(gitFile)
	if err != nil {
		return "", fmt.Errorf("read gitfile %s: %w", gitFile, err)
	}
	raw := parseGitDirLine(string(payload))
	if raw == "" {
		return "", nil
	}
	if !filepath.IsAbs(raw) {
		raw = filepath.Join(relativeTo, raw)
	}
	return filepath.Clean(raw), nil
}

func parseGitDirLine(body string) string {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 7 || !strings.EqualFold(trimmed[:7], "gitdir:") {
			continue
		}
		return strings.TrimSpace(trimmed[7:])
	}
	return ""
}

func detectRegistrations(root string) []Registration {
	var found []Registration
	product := filepath.Join(root, ".nicos", "product.yaml")
	if fileExists(product) {
		found = append(found, Registration{Kind: RegistrationProductYAML, Path: product})
	}
	productYML := filepath.Join(root, ".nicos", "product.yml")
	if fileExists(productYML) {
		found = append(found, Registration{Kind: RegistrationProductYAML, Path: productYML})
	}
	layoutDir := filepath.Join(root, ".nicos-catalog")
	corpusDir := filepath.Join(root, "catalog")
	hasCorpus := dirExists(corpusDir) && corpusHasEntities(corpusDir)
	// A leftover .nicos-catalog/cache from a prior reindex is not consent.
	if dirExists(layoutDir) && hasCorpus {
		found = append(found, Registration{Kind: RegistrationLayout, Path: layoutDir})
	} else if hasCorpus {
		found = append(found, Registration{Kind: RegistrationCorpus, Path: corpusDir})
	}
	return found
}

func fileExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func dirExists(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

func corpusHasEntities(dir string) bool {
	var has bool
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || has {
			return err
		}
		if entry.IsDir() {
			if entry.Type()&os.ModeSymlink != 0 {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".yaml", ".yml", ".json", ".md":
			//nolint:gosec // WalkDir supplies a path confined to the discovered corpus.
			payload, readErr := os.ReadFile(path)
			if readErr != nil {
				//nolint:nilerr // Consent discovery skips unreadable optional corpus files.
				return nil
			}
			if looksLikeEntity(string(payload)) {
				has = true
				return filepath.SkipDir
			}
		}
		return nil
	})
	return has
}

func looksLikeEntity(body string) bool {
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "id:") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "id:")) != ""
		}
		if strings.HasPrefix(trimmed, `"id"`) || strings.HasPrefix(trimmed, "'id'") {
			return true
		}
	}
	return false
}

// Classify assigns each clone to a report bucket. It does not emit records.
func Classify(settings Settings, clones []Clone) []Item {
	if !settings.Active() {
		return nil
	}
	profile := settings.Profile()
	var items []Item
	for _, clone := range clones {
		item := classifyClone(profile, clone)
		if item.Bucket == "" {
			continue
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Bucket == items[j].Bucket {
			return items[i].Path < items[j].Path
		}
		return items[i].Bucket < items[j].Bucket
	})
	return items
}

func classifyClone(profile string, clone Clone) Item {
	owner, repo, url, match := MatchingRemote(clone.Remotes, profile)
	registered := len(clone.Registrations) > 0
	item := Item{Path: clone.Path}
	if match {
		item.Repo = owner + "/" + repo
		item.Remote = url
		if registered {
			item.Bucket = BucketCollated
			item.Source = sourcesOf(clone.Registrations)
			return item
		}
		item.Bucket = BucketUnregistered
		return item
	}
	if !registered {
		return Item{}
	}
	item.Bucket = BucketWrongOwner
	item.Source = sourcesOf(clone.Registrations)
	if otherOwner, otherRepo, otherURL, ok := FirstGitHubRepo(clone.Remotes); ok {
		item.Repo = otherOwner + "/" + otherRepo
		item.Remote = otherURL
	}
	return item
}

func sourcesOf(regs []Registration) string {
	parts := make([]string, 0, len(regs))
	seen := map[RegistrationKind]struct{}{}
	for _, reg := range regs {
		if _, ok := seen[reg.Kind]; ok {
			continue
		}
		seen[reg.Kind] = struct{}{}
		parts = append(parts, string(reg.Kind))
	}
	return strings.Join(parts, ",")
}
