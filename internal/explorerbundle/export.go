// Package explorerbundle compiles deterministic public static Explorer output.
package explorerbundle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	explorerassets "github.com/nstranquist/nicos-catalog/explorer"
	"github.com/nstranquist/nicos-catalog/internal/explorerapi"
	"github.com/nstranquist/nicos-catalog/internal/explorercontract"
)

type Options struct {
	OutDir         string
	ForbiddenRoots []string
	ProductVersion string
}

// Export writes a complete public bundle through a unique sibling directory.
func Export(ctx context.Context, service *explorerapi.Service, options Options) (explorercontract.ExportReceipt, error) {
	if err := ctx.Err(); err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	if service == nil || service.Dataset().ProjectionMode != explorercontract.ProjectionPublic {
		return explorercontract.ExportReceipt{}, fmt.Errorf("static Explorer export requires the public projection")
	}
	out, err := validateTarget(options.OutDir, options.ForbiddenRoots)
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	exists, owned, err := targetState(out)
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	if exists && !owned {
		return explorercontract.ExportReceipt{}, fmt.Errorf("output directory is not an existing Explorer export")
	}
	parent := filepath.Dir(out)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	temp, err := os.MkdirTemp(parent, "."+filepath.Base(out)+".tmp-")
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	if err := os.Chmod(temp, 0o700); err != nil {
		_ = os.RemoveAll(temp)
		return explorercontract.ExportReceipt{}, err
	}
	keepTemp := true
	defer func() {
		if keepTemp {
			_ = os.RemoveAll(temp)
		}
	}()

	receipt, err := build(ctx, temp, service, options.ProductVersion)
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	if exists {
		equal, err := equalTrees(out, temp)
		if err != nil {
			return explorercontract.ExportReceipt{}, err
		}
		if equal {
			if err := os.RemoveAll(temp); err != nil {
				return explorercontract.ExportReceipt{}, err
			}
			keepTemp = false
			receipt.OutputChanged = false
			return receipt, nil
		}
	}
	if err := installDirectory(temp, out, exists); err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	keepTemp = false
	receipt.OutputChanged = true
	return receipt, nil
}

func build(ctx context.Context, root string, service *explorerapi.Service, productVersion string) (explorercontract.ExportReceipt, error) {
	if err := copyWeb(ctx, root); err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	dataset := service.Dataset()
	entities := explorercontract.EntityPage{Items: append([]explorercontract.Entity(nil), dataset.Entities...)}
	graph, graphMeta, err := service.Graph(explorerapi.GraphOptions{Mode: explorercontract.GraphAggregate})
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	health, healthMeta, err := service.Health("", 100)
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	data := map[string]struct {
		value any
		meta  explorercontract.Meta
	}{
		"entities": {value: entities, meta: explorercontract.Meta{Total: len(dataset.Entities), Truncated: false}},
		"graph":    {value: graph, meta: graphMeta},
		"health":   {value: health, meta: healthMeta},
		// Static search starts from the same closed entity bytes. The browser
		// builds its small local index; private source text never enters this file.
		"search": {value: entities, meta: explorercontract.Meta{Total: len(dataset.Entities), Truncated: false}},
	}
	digests := explorercontract.ContentDigests{}
	for _, name := range []string{"entities", "graph", "health", "search"} {
		item := data[name]
		payload, err := json.Marshal(item.value)
		if err != nil {
			return explorercontract.ExportReceipt{}, err
		}
		envelope := explorercontract.Envelope{
			SchemaVersion: explorercontract.SchemaVersion, OK: true, ProjectionMode: explorercontract.ProjectionPublic,
			SourceDigest: dataset.SourceDigest, Data: payload, Meta: item.meta,
		}
		encoded, err := marshalStable(envelope)
		if err != nil {
			return explorercontract.ExportReceipt{}, err
		}
		path := filepath.Join(root, "data", name+".json")
		if err := writeFile(path, encoded); err != nil {
			return explorercontract.ExportReceipt{}, err
		}
		switch name {
		case "entities":
			digests.Entities = digest(encoded)
		case "graph":
			digests.Graph = digest(encoded)
		case "health":
			digests.Health = digest(encoded)
		case "search":
			digests.Search = digest(encoded)
		}
	}
	manifest := explorercontract.Manifest{
		SchemaVersion: explorercontract.SchemaVersion, Generator: explorercontract.StaticGenerator,
		ProductVersion: productVersion, ProjectionMode: explorercontract.ProjectionPublic, SourceDigest: dataset.SourceDigest,
		EntityCount: len(dataset.Entities), EdgeCount: len(dataset.Edges), FindingCount: len(dataset.Findings), Content: digests,
	}
	manifestBytes, err := marshalStable(manifest)
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	if err := writeFile(filepath.Join(root, "data", "manifest.json"), manifestBytes); err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	files, err := relativeFiles(root)
	if err != nil {
		return explorercontract.ExportReceipt{}, err
	}
	return explorercontract.ExportReceipt{
		Files: files, Content: digests, EntityCount: len(dataset.Entities), EdgeCount: len(dataset.Edges),
		FindingCount: len(dataset.Findings), SourceDigest: dataset.SourceDigest,
	}, nil
}

func copyWeb(ctx context.Context, root string) error {
	files := explorerassets.FS()
	return fs.WalkDir(files, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		target := filepath.Join(root, filepath.FromSlash(name))
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		payload, err := fs.ReadFile(files, name)
		if err != nil {
			return err
		}
		return writeFile(target, payload)
	})
}

func validateTarget(raw string, forbidden []string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("output directory is required")
	}
	out, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return "", err
	}
	if out == string(filepath.Separator) || out == filepath.VolumeName(out)+string(filepath.Separator) {
		return "", fmt.Errorf("output directory is unsafe")
	}
	if err := rejectSymlinkChain(out); err != nil {
		return "", err
	}
	for _, root := range forbidden {
		if strings.TrimSpace(root) == "" {
			continue
		}
		absolute, err := filepath.Abs(filepath.Clean(root))
		if err != nil {
			return "", err
		}
		if sameOrWithin(absolute, out) {
			return "", fmt.Errorf("output directory overlaps a protected catalog directory")
		}
	}
	return out, nil
}

func rejectSymlinkChain(target string) error {
	current := filepath.VolumeName(target) + string(filepath.Separator)
	rel := strings.TrimPrefix(target, current)
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("output path contains a symlink")
		}
	}
	return nil
}

func targetState(target string) (exists, owned bool, err error) {
	info, err := os.Lstat(target)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return true, false, nil
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		return true, false, err
	}
	if len(entries) == 0 {
		return true, true, nil
	}
	payload, err := os.ReadFile(filepath.Join(target, "data", "manifest.json"))
	if err != nil {
		return true, false, nil
	}
	var manifest explorercontract.Manifest
	if json.Unmarshal(payload, &manifest) != nil {
		return true, false, nil
	}
	owned = manifest.Generator == explorercontract.StaticGenerator && manifest.SchemaVersion == explorercontract.SchemaVersion && manifest.ProjectionMode == explorercontract.ProjectionPublic
	return true, owned, nil
}

func installDirectory(temp, target string, targetExists bool) error {
	if !targetExists {
		return os.Rename(temp, target)
	}
	backup := target + ".previous-" + strings.TrimPrefix(filepath.Base(temp), "."+filepath.Base(target)+".tmp-")
	if _, err := os.Lstat(backup); !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("atomic backup path is not available")
	}
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(temp, target); err != nil {
		_ = os.Rename(backup, target)
		return err
	}
	if err := os.RemoveAll(backup); err != nil {
		return err
	}
	return nil
}

func equalTrees(left, right string) (bool, error) {
	leftFiles, err := relativeFiles(left)
	if err != nil {
		return false, err
	}
	rightFiles, err := relativeFiles(right)
	if err != nil {
		return false, err
	}
	if len(leftFiles) != len(rightFiles) {
		return false, nil
	}
	for i := range leftFiles {
		if leftFiles[i] != rightFiles[i] {
			return false, nil
		}
		a, err := os.ReadFile(filepath.Join(left, filepath.FromSlash(leftFiles[i])))
		if err != nil {
			return false, err
		}
		b, err := os.ReadFile(filepath.Join(right, filepath.FromSlash(rightFiles[i])))
		if err != nil {
			return false, err
		}
		if !bytes.Equal(a, b) {
			return false, nil
		}
	}
	return true, nil
}

func relativeFiles(root string) ([]string, error) {
	files := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

func sameOrWithin(parent, child string) bool {
	rel, err := filepath.Rel(parent, child)
	return err == nil && (rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))))
}

func writeFile(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}
func marshalStable(value any) ([]byte, error) {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(payload, '\n'), nil
}
func digest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}
