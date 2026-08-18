package hostcollate

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	catalog "github.com/nstranquist/nicos-catalog"
)

// Report is the dry-run / apply result. Unregistered and wrong-owner clones
// appear only as buckets, never as invented entities.
type Report struct {
	Enabled        bool   `json:"enabled"`
	Profile        string `json:"profile"`
	Applied        bool   `json:"applied"`
	Collated       []Item `json:"collated"`
	Unregistered   []Item `json:"unregistered"`
	WrongOwner     []Item `json:"wrong_owner"`
	Missing        []Item `json:"missing"`
	Rejected       []Item `json:"rejected"`
	Duplicates     []Item `json:"duplicates"`
	Gaps           []Item `json:"gaps,omitempty"`
	RecordCount    int    `json:"record_count"`
	IndexPath      string `json:"index_path,omitempty"`
	SnapshotPath   string `json:"snapshot_path,omitempty"`
	SnapshotDigest string `json:"snapshot_digest,omitempty"`
	Walked         int    `json:"walked"`
	WalkCapped     bool   `json:"walk_capped"`
	Reason         string `json:"reason,omitempty"`
}

// Options configure one collate run. Home is injected so tests never rewrite HOME.
type Options struct {
	HostRoot       string
	ConfigDir      string
	CacheDir       string
	CorpusDir      string
	Sidecars       string
	Home           string
	Apply          bool
	WriteSnapshot  bool
	FromSnapshot   bool
	ProfileRepos   []string
	ProfileLister  ProfileRepoLister
	Walk           func(roots []string, opts WalkOptions) ([]Clone, error)
	EnrollManifest string
}

// ProfileRepoLister returns owner/repo names for the opted-in profile.
// Tests inject a static list; a live adapter is optional.
type ProfileRepoLister func(ctx context.Context, profile string) ([]string, error)

// Discover classifies local clones and emits portable records. It never writes.
func Discover(ctx context.Context, opts Options) (Report, []catalog.Record, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, nil, err
	}
	if opts.FromSnapshot {
		snap, snapErr := ReadSnapshot(opts.CacheDir)
		if snapErr != nil {
			return Report{}, nil, snapErr
		}
		if opts.Apply && len(snap.Records) == 0 {
			return snap.Report, nil, fmt.Errorf("from-snapshot cannot apply: snapshot has no stored records")
		}
		if opts.Apply {
			if err := rejectHostCollisions(ctx, opts.CorpusDir, snap.Records); err != nil {
				return snap.Report, snap.Records, err
			}
		}
		return snap.Report, snap.Records, nil
	}
	settings, err := LoadSettings(SettingsPath(opts.ConfigDir))
	if err != nil {
		return Report{}, nil, err
	}
	report := newReport(settings.GitHub.Collation.Enabled, settings.Profile())
	if !settings.Active() {
		if !settings.GitHub.Collation.Enabled {
			report.Reason = "collation disabled"
		} else {
			report.Reason = "profile missing"
		}
		return report, nil, nil
	}
	roots, err := settings.ExpandRoots(opts.Home, opts.HostRoot)
	if err != nil {
		return report, nil, err
	}
	walk := opts.Walk
	if walk == nil {
		walk = WalkRoots
	}
	policy := settings.WalkPolicy()
	policy.Home = opts.Home
	var walkCapped bool
	policy.Capped = &walkCapped
	clones, err := walk(roots, policy)
	if err != nil {
		return report, nil, err
	}
	report.Walked = len(clones)
	report.WalkCapped = walkCapped
	items := Classify(settings, clones)
	byPath := map[string]Clone{}
	for _, clone := range clones {
		byPath[clone.Path] = clone
	}
	var records []catalog.Record
	for i, item := range items {
		if item.Bucket != BucketCollated {
			continue
		}
		emitted, emitErr := EmitRecords(ctx, item, byPath[item.Path])
		if emitErr != nil {
			return report, nil, emitErr
		}
		accepted := acceptedRecords(emitted)
		ids := make([]string, 0, len(accepted))
		for _, record := range accepted {
			ids = append(ids, record.Entity.ID)
		}
		sort.Strings(ids)
		if len(accepted) == 0 {
			item.Bucket = BucketRejected
			item.EntityIDs = nil
			items[i] = item
			continue
		}
		item.EntityIDs = ids
		items[i] = item
		records = append(records, accepted...)
	}
	missing, missErr := missingClones(ctx, settings, opts, clones)
	if missErr != nil {
		return report, records, missErr
	}
	report.Missing = missing
	if opts.EnrollManifest != "" {
		gaps, gapErr := EnrollmentGaps(clones, opts.EnrollManifest)
		if gapErr != nil {
			return report, records, gapErr
		}
		report.Gaps = gaps
	}
	items, records = applyDuplicatePolicy(items, records)
	report = bucketReport(report, items, records)
	if err := rejectHostCollisions(ctx, opts.CorpusDir, records); err != nil {
		return report, records, err
	}
	if len(report.Duplicates) > 0 {
		id := report.Duplicates[0].Repo
		if len(report.Duplicates[0].EntityIDs) > 0 {
			id = report.Duplicates[0].EntityIDs[0]
		}
		return report, records, fmt.Errorf("duplicate entity ids: %s", id)
	}
	return report, records, nil
}

// Run loads settings, classifies local clones, and optionally rebuilds the
// host derived index. It never clones, fetches, pulls, or writes scanned repos.
func Run(ctx context.Context, opts Options) (Report, error) {
	report, records, err := Discover(ctx, opts)
	if err != nil {
		return report, err
	}
	if !opts.Apply {
		if opts.WriteSnapshot {
			if err := attachSnapshot(opts.CacheDir, &report, records); err != nil {
				return report, err
			}
		}
		return report, nil
	}
	indexPath, err := applyRecords(ctx, opts, records)
	if err != nil {
		return report, err
	}
	report.Applied = true
	report.IndexPath = indexPath
	if err := attachSnapshot(opts.CacheDir, &report, records); err != nil {
		return report, err
	}
	return report, nil
}

func attachSnapshot(cacheDir string, report *Report, records []catalog.Record) error {
	snap, err := WriteSnapshot(cacheDir, *report, records)
	if err != nil {
		return err
	}
	report.SnapshotPath = snap.Path
	report.SnapshotDigest = snap.Digest
	return nil
}

func newReport(enabled bool, profile string) Report {
	return Report{
		Enabled:      enabled,
		Profile:      profile,
		Collated:     []Item{},
		Unregistered: []Item{},
		WrongOwner:   []Item{},
		Missing:      []Item{},
		Rejected:     []Item{},
		Duplicates:   []Item{},
		Gaps:         []Item{},
	}
}

func bucketReport(base Report, items []Item, records []catalog.Record) Report {
	base.Collated = []Item{}
	base.Unregistered = []Item{}
	base.WrongOwner = []Item{}
	base.Rejected = []Item{}
	base.Duplicates = []Item{}
	if base.Missing == nil {
		base.Missing = []Item{}
	}
	if base.Gaps == nil {
		base.Gaps = []Item{}
	}
	for _, item := range items {
		switch item.Bucket {
		case BucketCollated:
			base.Collated = append(base.Collated, item)
		case BucketUnregistered:
			base.Unregistered = append(base.Unregistered, item)
		case BucketWrongOwner:
			base.WrongOwner = append(base.WrongOwner, item)
		case BucketRejected:
			base.Rejected = append(base.Rejected, item)
		case BucketDuplicate:
			base.Duplicates = append(base.Duplicates, item)
		}
	}
	base.RecordCount = len(records)
	return base
}

func applyDuplicatePolicy(items []Item, records []catalog.Record) ([]Item, []catalog.Record) {
	seen := map[string]string{}
	var kept []catalog.Record
	dropped := map[string]struct{}{}
	for _, record := range records {
		id := record.Entity.ID
		if _, ok := seen[id]; ok {
			dropped[id] = struct{}{}
			continue
		}
		seen[id] = record.Source
		kept = append(kept, record)
	}
	if len(dropped) == 0 {
		return items, kept
	}
	for i, item := range items {
		if item.Bucket != BucketCollated {
			continue
		}
		var remain []string
		var lost []string
		for _, id := range item.EntityIDs {
			if _, hit := dropped[id]; hit {
				lost = append(lost, id)
			} else {
				remain = append(remain, id)
			}
		}
		if len(remain) == 0 && len(lost) > 0 {
			item.Bucket = BucketDuplicate
			items[i] = item
			continue
		}
		item.EntityIDs = remain
		items[i] = item
	}
	return items, kept
}

func rejectHostCollisions(ctx context.Context, corpusDir string, records []catalog.Record) error {
	if !corpusUsable(corpusDir) {
		return nil
	}
	provided, err := (catalog.FilesystemProvider{ProviderName: "host-filesystem", Strict: true}).Provide(ctx, catalog.Layout{CorpusDir: corpusDir})
	if err != nil {
		return err
	}
	host := map[string]string{}
	for _, record := range provided {
		host[record.Entity.ID] = record.Source
	}
	for _, record := range records {
		if first, ok := host[record.Entity.ID]; ok {
			return &catalog.DuplicateIDError{
				EntityID: record.Entity.ID,
				First:    catalog.RecordOrigin{Provider: "host-filesystem", Source: first},
				Second:   catalog.RecordOrigin{Provider: ProviderName, Source: record.Source},
			}
		}
	}
	return nil
}

func rejectDuplicateIDs(records []catalog.Record) error {
	seen := map[string]string{}
	for _, record := range records {
		id := record.Entity.ID
		if first, ok := seen[id]; ok {
			return &catalog.DuplicateIDError{
				EntityID: id,
				First:    catalog.RecordOrigin{Provider: ProviderName, Source: first},
				Second:   catalog.RecordOrigin{Provider: ProviderName, Source: record.Source},
			}
		}
		seen[id] = record.Source
	}
	return nil
}

func applyRecords(ctx context.Context, opts Options, records []catalog.Record) (string, error) {
	layout, err := (catalog.Layout{
		CorpusDir:      opts.CorpusDir,
		ConfigDir:      opts.ConfigDir,
		CacheDir:       opts.CacheDir,
		SidecarDataDir: opts.Sidecars,
	}).Resolve(opts.HostRoot)
	if err != nil {
		return "", err
	}
	var providers []catalog.Provider
	if corpusUsable(layout.CorpusDir) {
		providers = append(providers, catalog.FilesystemProvider{ProviderName: "host-filesystem", Strict: true})
	}
	if len(records) > 0 {
		providers = append(providers, RecordsProvider{ProviderName: ProviderName, Records: records})
	}
	if len(providers) == 0 {
		providers = append(providers, catalog.StaticProvider{ProviderName: "github-local-empty"})
	}
	engine, err := catalog.New(layout, catalog.WithProviders(providers...))
	if err != nil {
		return "", err
	}
	reindexed, err := engine.Reindex(ctx)
	if err != nil {
		return "", err
	}
	return reindexed.IndexPath, nil
}

func corpusUsable(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// FormatReport renders a stable human report.
func FormatReport(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "collate: profile=%q enabled=%t records=%d walked=%d capped=%t\n", report.Profile, report.Enabled, report.RecordCount, report.Walked, report.WalkCapped)
	if report.Reason != "" {
		fmt.Fprintf(&b, "reason: %s\n", report.Reason)
	}
	writeBucket(&b, "collated", report.Collated)
	writeBucket(&b, "unregistered", report.Unregistered)
	writeBucket(&b, "wrong_owner", report.WrongOwner)
	writeBucket(&b, "missing", report.Missing)
	writeBucket(&b, "rejected", report.Rejected)
	writeBucket(&b, "duplicates", report.Duplicates)
	writeBucket(&b, "gaps", report.Gaps)
	if report.Applied {
		fmt.Fprintf(&b, "applied: %s\n", report.IndexPath)
	}
	return b.String()
}

func writeBucket(b *strings.Builder, name string, items []Item) {
	fmt.Fprintf(b, "%s: %d\n", name, len(items))
	for _, item := range items {
		ids := strings.Join(item.EntityIDs, ",")
		fmt.Fprintf(b, "  %s\t%s\t%s\t%s\n", item.Path, item.Repo, item.Source, ids)
	}
}
