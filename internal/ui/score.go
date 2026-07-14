package ui

import (
	"sort"

	"github.com/sahilm/fuzzy"
)

func normalizeScores(scores map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(scores))
	maxScore := 0.0
	for _, v := range scores {
		if v > maxScore {
			maxScore = v
		}
	}
	if maxScore == 0 {
		return out
	}
	for k, v := range scores {
		out[k] = v / maxScore
	}
	return out
}

// combineScore weights fuzzy 60%, frecency 40%.
func combineScore(normFuzzyScore, normFrecency float64) float64 {
	return normFuzzyScore*0.6 + normFrecency*0.4
}

// scoreAndSort takes items carrying raw fuzzy scores, normalizes them to
// [0,1] within the set, blends in frecency, and sorts best-first.
func (m *model) scoreAndSort(items []scoredItem) []scoredItem {
	maxRaw := 0.0
	for _, it := range items {
		if it.score > maxRaw {
			maxRaw = it.score
		}
	}
	for i, it := range items {
		normFuzzy := 0.0
		if maxRaw > 0 {
			normFuzzy = it.score / maxRaw
		}
		items[i].score = combineScore(normFuzzy, m.normFrec[it.base.candidate.AbsPath])
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].score > items[j].score
	})
	return items
}

// filterScored ranks pool against query: fuzzy+frecency blend for a non-empty
// query, frecency alone otherwise.
func (m *model) filterScored(pool []baseItem, query string) []scoredItem {
	if query == "" {
		result := make([]scoredItem, 0, len(pool))
		for _, item := range pool {
			result = append(result, scoredItem{base: item, score: m.normFrec[item.candidate.AbsPath]})
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].score > result[j].score
		})
		return result
	}
	// fuzzy match on RelPath — avoids ANSI escape codes in display strings
	keys := make([]string, len(pool))
	for i, item := range pool {
		keys[i] = item.candidate.RelPath
	}
	var result []scoredItem
	for _, match := range fuzzy.Find(query, keys) {
		result = append(result, scoredItem{
			base:    pool[match.Index],
			score:   float64(match.Score),
			matches: match.MatchedIndexes,
		})
	}
	return m.scoreAndSort(result)
}

func (m *model) rebuildFiltered() {
	pool := make([]baseItem, 0, len(m.all))
	for _, item := range m.all {
		if m.switchOnly && !item.active {
			continue
		}
		if !m.matchesView(item) {
			continue
		}
		pool = append(pool, item)
	}
	m.filtered = m.filterScored(pool, m.tiQuery.Value())
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

func (m *model) rebuildDestFiltered() {
	// pool: non-repo candidates from m.all (projects + tmp projects)
	pool := make([]baseItem, 0, len(m.all))
	for _, item := range m.all {
		if !item.candidate.IsRepo {
			pool = append(pool, item)
		}
	}
	m.clone.destFiltered = m.filterScored(pool, m.clone.tiDest.Value())
	if m.clone.destCursor >= len(m.clone.destFiltered) {
		m.clone.destCursor = 0
	}
}

func (m *model) rebuildCleanFiltered() {
	all := m.tmpItems()
	query := m.clean.tiQuery.Value()
	if query == "" {
		m.clean.filtered = make([]scoredItem, len(all))
		for i, item := range all {
			m.clean.filtered[i] = scoredItem{base: item}
		}
	} else {
		keys := make([]string, len(all))
		for i, item := range all {
			keys[i] = item.candidate.RelPath
		}
		m.clean.filtered = nil
		for _, match := range fuzzy.Find(query, keys) {
			m.clean.filtered = append(m.clean.filtered, scoredItem{base: all[match.Index], matches: match.MatchedIndexes})
		}
	}
	if m.clean.cursor >= len(m.clean.filtered) {
		m.clean.cursor = 0
	}
}

func (m *model) matchesView(item baseItem) bool {
	switch m.view {
	case viewProject:
		return !item.candidate.IsRepo && !item.candidate.IsTmp
	case viewRepo:
		return item.candidate.IsRepo
	case viewTmp:
		return item.candidate.IsTmp
	default: // viewAll
		return true
	}
}
