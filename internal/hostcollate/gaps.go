package hostcollate

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

type enrollManifest struct {
	Projects []struct {
		ID string `yaml:"id"`
	} `yaml:"projects"`
}

// EnrollmentGaps lists locally registered product IDs absent from an
// external-projects.yaml copy. It never writes the file.
func EnrollmentGaps(clones []Clone, manifestPath string) ([]Item, error) {
	enrolled, err := loadEnrolledIDs(manifestPath)
	if err != nil {
		return nil, err
	}
	var gaps []Item
	seen := map[string]struct{}{}
	for _, clone := range clones {
		if len(clone.Registrations) == 0 {
			continue
		}
		ids := registeredIDs(clone)
		for _, id := range ids {
			if _, ok := enrolled[id]; ok {
				continue
			}
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			gaps = append(gaps, Item{
				Path:      clone.Path,
				Repo:      id,
				Bucket:    BucketGap,
				Source:    sourcesOf(clone.Registrations),
				EntityIDs: []string{id},
			})
		}
	}
	sort.Slice(gaps, func(i, j int) bool { return gaps[i].Repo < gaps[j].Repo })
	return gaps, nil
}

func loadEnrolledIDs(path string) (map[string]struct{}, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read enroll manifest: %w", err)
	}
	var man enrollManifest
	if err := yaml.Unmarshal(payload, &man); err != nil {
		return nil, fmt.Errorf("decode enroll manifest: %w", err)
	}
	out := map[string]struct{}{}
	for _, project := range man.Projects {
		id := strings.TrimSpace(project.ID)
		if id != "" {
			out[id] = struct{}{}
		}
	}
	return out, nil
}

func registeredIDs(clone Clone) []string {
	var ids []string
	seen := map[string]struct{}{}
	add := func(id string) {
		id = strings.TrimSpace(id)
		if id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, reg := range clone.Registrations {
		switch reg.Kind {
		case RegistrationProductYAML:
			payload, err := os.ReadFile(reg.Path)
			if err != nil {
				continue
			}
			var manifest productManifest
			if yaml.Unmarshal(payload, &manifest) != nil {
				continue
			}
			add(manifest.ID)
		case RegistrationCorpus:
			for _, id := range corpusEntityIDs(reg.Path) {
				add(id)
			}
		case RegistrationLayout:
			for _, id := range corpusEntityIDs(filepath.Join(clone.Path, "catalog")) {
				add(id)
			}
		}
	}
	return ids
}

func corpusEntityIDs(dir string) []string {
	var ids []string
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(entry.Name())) {
		case ".yaml", ".yml":
		default:
			return nil
		}
		payload, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		var manifest struct {
			ID string `yaml:"id"`
		}
		if yaml.Unmarshal(payload, &manifest) != nil {
			return nil
		}
		if id := strings.TrimSpace(manifest.ID); id != "" {
			ids = append(ids, id)
		}
		return nil
	})
	return ids
}
