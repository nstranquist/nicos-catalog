package hostcollate

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	catalog "github.com/nstranquist/nicos-catalog"
	"go.yaml.in/yaml/v3"
)

// ProviderName is the Record.Provider for every collated entity.
const ProviderName = "github-local"

type productManifest struct {
	ID          string             `yaml:"id"`
	Name        string             `yaml:"name"`
	Kind        string             `yaml:"kind"`
	Surface     string             `yaml:"surface"`
	Status      string             `yaml:"status"`
	Description string             `yaml:"description"`
	Entrypoint  string             `yaml:"entrypoint"`
	Owner       string             `yaml:"owner"`
	Tags        []string           `yaml:"tags"`
	Refs        []catalog.Ref      `yaml:"refs"`
	PublicURL   string             `yaml:"public_url"`
	Visibility  catalog.Visibility `yaml:"visibility"`
}

// EmitRecords turns a collated clone into portable engine records. Other
// buckets must not call this: they never invent entities.
func EmitRecords(ctx context.Context, item Item, clone Clone) ([]catalog.Record, error) {
	if item.Bucket != BucketCollated {
		return nil, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var records []catalog.Record
	for _, reg := range clone.Registrations {
		switch reg.Kind {
		case RegistrationProductYAML:
			record, err := emitProductYAML(item, reg.Path)
			if err != nil {
				return nil, err
			}
			if record != nil {
				records = append(records, *record)
			}
		case RegistrationCorpus:
			emitted, err := emitCorpus(ctx, item, reg.Path)
			if err != nil {
				return nil, err
			}
			records = append(records, emitted...)
		case RegistrationLayout:
			emitted, err := emitLayout(ctx, item, clone.Path)
			if err != nil {
				return nil, err
			}
			records = append(records, emitted...)
		}
	}
	return dedupeRecords(records), nil
}

func emitProductYAML(item Item, path string) (*catalog.Record, error) {
	//nolint:gosec // Registration discovery supplies the consent-manifest path.
	payload, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read product manifest %s: %w", path, err)
	}
	var manifest productManifest
	if err := yaml.Unmarshal(payload, &manifest); err != nil {
		return nil, fmt.Errorf("decode product manifest %s: %w", path, err)
	}
	id := strings.TrimSpace(manifest.ID)
	if id == "" {
		return nil, nil
	}
	name := strings.TrimSpace(manifest.Name)
	if name == "" {
		name = id
	}
	kind := strings.TrimSpace(manifest.Kind)
	if kind == "" {
		kind = "product"
	}
	entity := catalog.Entity{
		ID:          id,
		Name:        name,
		Kind:        kind,
		Surface:     strings.TrimSpace(manifest.Surface),
		Status:      strings.TrimSpace(manifest.Status),
		Description: strings.TrimSpace(manifest.Description),
		Entrypoint:  strings.TrimSpace(manifest.Entrypoint),
		Owner:       strings.TrimSpace(manifest.Owner),
		Tags:        append([]string(nil), manifest.Tags...),
		Refs:        append([]catalog.Ref(nil), manifest.Refs...),
		PublicURL:   strings.TrimSpace(manifest.PublicURL),
		Visibility:  manifest.Visibility,
	}
	source := provenance(item.Repo, relSource(item.Path, path))
	return &catalog.Record{
		Entity:   entity,
		Provider: ProviderName,
		Source:   source,
	}, nil
}

func emitCorpus(ctx context.Context, item Item, corpusDir string) ([]catalog.Record, error) {
	provider := catalog.FilesystemProvider{ProviderName: ProviderName}
	records, err := provider.Provide(ctx, catalog.Layout{CorpusDir: corpusDir})
	if err != nil {
		return nil, fmt.Errorf("corpus %s: %w", corpusDir, err)
	}
	return stampProvenance(item, records), nil
}

func emitLayout(ctx context.Context, item Item, repoRoot string) ([]catalog.Record, error) {
	layout, err := catalog.DefaultLayout(repoRoot).Resolve(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("layout %s: %w", repoRoot, err)
	}
	if !dirExists(layout.CorpusDir) {
		return nil, nil
	}
	provider := catalog.FilesystemProvider{ProviderName: ProviderName}
	records, err := provider.Provide(ctx, layout)
	if err != nil {
		return nil, fmt.Errorf("layout corpus %s: %w", layout.CorpusDir, err)
	}
	return stampProvenance(item, records), nil
}

func stampProvenance(item Item, records []catalog.Record) []catalog.Record {
	stamped := make([]catalog.Record, 0, len(records))
	for _, record := range records {
		record.Provider = ProviderName
		record.Source = provenance(item.Repo, record.Source)
		stamped = append(stamped, record)
	}
	return stamped
}

func provenance(repo, source string) string {
	repo = strings.TrimSpace(repo)
	source = strings.TrimSpace(source)
	if repo == "" {
		return ProviderName + ":" + source
	}
	return ProviderName + ":" + repo + ":" + source
}

func relSource(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func dedupeRecords(records []catalog.Record) []catalog.Record {
	seen := map[string]struct{}{}
	var out []catalog.Record
	for _, record := range records {
		key := record.Entity.ID + "\x00" + record.Source
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, record)
	}
	return out
}

// RecordsProvider feeds already-built records into the engine without
// rewriting provenance the way StaticProvider would.
type RecordsProvider struct {
	ProviderName string
	Records      []catalog.Record
}

// Name reports the provider identity.
func (p RecordsProvider) Name() string {
	if strings.TrimSpace(p.ProviderName) == "" {
		return ProviderName
	}
	return strings.TrimSpace(p.ProviderName)
}

// Provide returns a copy of the configured records.
func (p RecordsProvider) Provide(_ context.Context, _ catalog.Layout) ([]catalog.Record, error) {
	out := make([]catalog.Record, len(p.Records))
	copy(out, p.Records)
	return out, nil
}
