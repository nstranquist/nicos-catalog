package catalog

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
)

var tokenPattern = regexp.MustCompile(`[a-z0-9][a-z0-9._+-]*`)

type SearchOptions struct {
	Limit int
	Kinds []string
}

type SearchResult struct {
	Entity       Entity   `json:"entity"`
	Score        float64  `json:"score"`
	MatchedTerms []string `json:"matched_terms"`
}

// Search performs BM25 full-text retrieval over the deterministic local index.
func (e *Engine) Search(query string, options SearchOptions) ([]SearchResult, error) {
	index, err := e.LoadIndex()
	if err != nil {
		return nil, err
	}
	terms := normalizeStrings(tokenize(query))
	if len(terms) == 0 {
		return nil, fmt.Errorf("search query must contain at least one term")
	}
	limit := options.Limit
	if limit <= 0 {
		limit = 10
	}
	kinds := map[string]struct{}{}
	for _, kind := range options.Kinds {
		kinds[strings.ToLower(strings.TrimSpace(kind))] = struct{}{}
	}
	entities := make(map[string]Entity, len(index.Entities))
	for _, entity := range index.Entities {
		entities[entity.ID] = entity
	}
	documentFrequency := map[string]int{}
	for _, document := range index.Documents {
		for _, term := range terms {
			if document.TermFrequency[term] > 0 {
				documentFrequency[term]++
			}
		}
	}
	const k1 = 1.2
	const b = 0.75
	averageLength := index.AverageDocumentLength
	if averageLength == 0 {
		averageLength = 1
	}
	var results []SearchResult
	for _, document := range index.Documents {
		entity := entities[document.EntityID]
		if len(kinds) > 0 {
			if _, ok := kinds[strings.ToLower(entity.Kind)]; !ok {
				continue
			}
		}
		var score float64
		var matched []string
		for _, term := range terms {
			tf := document.TermFrequency[term]
			if tf == 0 {
				continue
			}
			df := documentFrequency[term]
			idf := math.Log(1 + (float64(len(index.Documents)-df)+0.5)/(float64(df)+0.5))
			denominator := float64(tf) + k1*(1-b+b*float64(document.Length)/averageLength)
			score += idf * (float64(tf) * (k1 + 1)) / denominator
			matched = append(matched, term)
		}
		if score == 0 {
			continue
		}
		lowerID := strings.ToLower(entity.ID)
		lowerName := strings.ToLower(entity.Name)
		for _, term := range terms {
			if lowerID == term || lowerName == term {
				score += 4
			} else if strings.Contains(lowerID, term) || strings.Contains(lowerName, term) {
				score += 1.5
			}
		}
		results = append(results, SearchResult{Entity: entity, Score: score, MatchedTerms: matched})
	}
	sort.Slice(results, func(i, j int) bool {
		if math.Abs(results[i].Score-results[j].Score) < 1e-9 {
			return results[i].Entity.ID < results[j].Entity.ID
		}
		return results[i].Score > results[j].Score
	})
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func searchableText(entity Entity) string {
	parts := []string{entity.ID, entity.Name, entity.Kind, entity.Surface, entity.Status, entity.Description, entity.Owner}
	parts = append(parts, entity.Tags...)
	for _, ref := range entity.Refs {
		parts = append(parts, ref.Kind, ref.Target)
	}
	return strings.Join(parts, " ")
}

func tokenize(value string) []string {
	return tokenPattern.FindAllString(strings.ToLower(value), -1)
}
