package hostcollate

import (
	"context"
	"sort"
	"strings"
)

func missingClones(ctx context.Context, settings Settings, opts Options, clones []Clone) ([]Item, error) {
	if !settings.CompareEnabled() && len(opts.ProfileRepos) == 0 && opts.ProfileLister == nil {
		return []Item{}, nil
	}
	names, err := profileRepoNames(ctx, settings.Profile(), opts)
	if err != nil {
		return nil, err
	}
	local := map[string]struct{}{}
	for _, clone := range clones {
		owner, repo, _, ok := MatchingRemote(clone.Remotes, settings.Profile())
		if !ok {
			if cloneRepo := strings.ToLower(strings.TrimSpace(repoNameFromClone(clone))); cloneRepo != "" {
				local[cloneRepo] = struct{}{}
			}
			continue
		}
		local[strings.ToLower(owner+"/"+repo)] = struct{}{}
	}
	var missing []Item
	for _, name := range names {
		key := strings.ToLower(strings.TrimSpace(name))
		if key == "" {
			continue
		}
		if _, ok := local[key]; ok {
			continue
		}
		missing = append(missing, Item{Repo: name, Bucket: BucketMissing})
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Repo < missing[j].Repo })
	return missing, nil
}

func profileRepoNames(ctx context.Context, profile string, opts Options) ([]string, error) {
	if len(opts.ProfileRepos) > 0 {
		return append([]string(nil), opts.ProfileRepos...), nil
	}
	if opts.ProfileLister == nil {
		return nil, nil
	}
	return opts.ProfileLister(ctx, profile)
}

func repoNameFromClone(clone Clone) string {
	owner, repo, _, ok := FirstGitHubRepo(clone.Remotes)
	if !ok {
		return ""
	}
	return owner + "/" + repo
}
