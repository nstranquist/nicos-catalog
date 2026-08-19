// Package explorerinit creates a safe starter corpus without overwriting files.
package explorerinit

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

type Options struct {
	Root     string
	Template string
	DryRun   bool
}

// Run preflights the complete plan before writing any file.
func Run(options Options) (explorercontract.InitReceipt, error) {
	template := strings.TrimSpace(options.Template)
	if template == "" {
		template = "minimal"
	}
	files, ok := templates[template]
	if !ok {
		return explorercontract.InitReceipt{}, fmt.Errorf("template must be minimal or sample")
	}
	root := options.Root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return explorercontract.InitReceipt{}, err
	}
	info, err := os.Lstat(absolute)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return explorercontract.InitReceipt{}, fmt.Errorf("init root must be an existing directory")
	}
	receipt := explorercontract.InitReceipt{Template: template, DryRun: options.DryRun, Planned: []string{}, Present: []string{}, Written: []string{}, Blocked: []string{}}
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, rel := range paths {
		receipt.Planned = append(receipt.Planned, filepath.ToSlash(rel))
		target, err := safeTarget(absolute, rel)
		if err != nil {
			return receipt, err
		}
		payload, err := os.ReadFile(target)
		switch {
		case err == nil && bytes.Equal(payload, files[rel]):
			receipt.Present = append(receipt.Present, filepath.ToSlash(rel))
		case err == nil:
			receipt.Blocked = append(receipt.Blocked, filepath.ToSlash(rel))
		case errors.Is(err, os.ErrNotExist):
		default:
			return receipt, err
		}
	}
	if len(receipt.Blocked) > 0 {
		return receipt, fmt.Errorf("init refused files with different content")
	}
	if options.DryRun {
		return receipt, nil
	}
	created := make([]string, 0)
	rollback := func() {
		for _, path := range created {
			_ = os.Remove(path)
		}
	}
	for _, rel := range paths {
		if contains(receipt.Present, filepath.ToSlash(rel)) {
			continue
		}
		target, err := safeTarget(absolute, rel)
		if err != nil {
			rollback()
			return receipt, err
		}
		mode := os.FileMode(0o755)
		if strings.HasPrefix(filepath.ToSlash(rel), ".nicos-catalog/") {
			mode = 0o700
		}
		if err := os.MkdirAll(filepath.Dir(target), mode); err != nil {
			rollback()
			return receipt, err
		}
		file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			rollback()
			return receipt, fmt.Errorf("init target changed during write")
		}
		_, writeErr := file.Write(files[rel])
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			_ = os.Remove(target)
			rollback()
			if writeErr != nil {
				return receipt, writeErr
			}
			return receipt, closeErr
		}
		created = append(created, target)
		receipt.Written = append(receipt.Written, filepath.ToSlash(rel))
	}
	return receipt, nil
}

func safeTarget(root, rel string) (string, error) {
	if filepath.IsAbs(rel) || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("unsafe init path")
	}
	target := filepath.Join(root, rel)
	current := root
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("init path contains an unsafe component")
		}
	}
	return target, nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

var templates = map[string]map[string][]byte{
	"minimal": {
		".nicos-catalog/settings.yaml": []byte("github:\n  collation:\n    enabled: false\n"),
		"catalog/example.md":           []byte("---\nid: service.example\nname: Example Service\nkind: service\nstatus: proposed\nvisibility: public\ntags: [example]\n---\n\nDescribe what this service does and who it helps.\n"),
	},
	"sample": {
		".nicos-catalog/settings.yaml":           []byte("github:\n  collation:\n    enabled: false\n"),
		"catalog/system.orchard.md":              []byte("---\nid: system.orchard\nname: Orchard Platform\nkind: system\nstatus: shipped\nvisibility: public\ntags: [demo, platform]\nrefs:\n  - kind: contains\n    target: service.seed-api\n  - kind: contains\n    target: web.grove-console\n---\n\nA synthetic developer platform that keeps ownership and relationships clear.\n"),
		"catalog/service.seed-api.yaml":          []byte("id: service.seed-api\nname: Seed API\nkind: service\nstatus: shipped\nvisibility: public\ndescription: A synthetic Go service for seed inventory.\ntags: [demo, go]\nrefs:\n  - kind: serves\n    target: web.grove-console\n"),
		"catalog/service.pollinator-worker.yaml": []byte("id: service.pollinator-worker\nname: Pollinator Worker\nkind: service\nstatus: beta\nvisibility: public\ndescription: Processes synthetic orchard events.\ntags: [demo, worker]\nrefs:\n  - kind: depends_on\n    target: service.seed-api\n"),
		"catalog/web.grove-console.json":         []byte("{\n  \"id\": \"web.grove-console\",\n  \"name\": \"Grove Console\",\n  \"kind\": \"web-app\",\n  \"status\": \"beta\",\n  \"visibility\": \"public\",\n  \"description\": \"Explores the synthetic orchard catalog.\",\n  \"tags\": [\"demo\", \"react\"]\n}\n"),
		"catalog/library.rootstock.yaml":         []byte("id: library.rootstock\nname: Rootstock Library\nkind: library\nstatus: stable\nvisibility: public\ndescription: Shared types for the synthetic orchard.\ntags: [demo, library]\nrefs:\n  - kind: used_by\n    target: service.seed-api\n"),
		"catalog/doc.growing-guide.md":           []byte("---\nid: doc.growing-guide\nname: Growing Guide\nkind: document\nstatus: maintained\nvisibility: public\ntags: [demo, docs]\nrefs:\n  - kind: documents\n    target: system.orchard\n---\n\nA synthetic onboarding guide for Orchard Platform contributors.\n"),
		"catalog/telemetry.private-sample.yaml":  []byte("id: telemetry.private-sample\nname: Private Sample\nkind: telemetry\nstatus: active\nvisibility: private\ndescription: A synthetic private record that proves public projection is closed.\ntags: [demo]\nannotations:\n  local_only: not projected\n"),
	},
}
