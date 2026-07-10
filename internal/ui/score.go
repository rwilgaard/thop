package ui

import (
	"sort"

	"github.com/sahilm/fuzzy"
)

func normalizeScores(scores map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(scores))
	max := 0.0
	for _, v := range scores {
		if v > max {
			max = v
		}
	}
	if max == 0 {
		return out
	}
	for k, v := range scores {
		out[k] = v / max
	}
	return out
}

// combineScore weights fuzzy 60%, frecency 40%.
func combineScore(normFuzzyScore, normFrecency float64) float64 {
	return normFuzzyScore*0.6 + normFrecency*0.4
}

func (m *model) scoreAndSort(pending []pendingItem) []scoredItem {
	maxRaw := 0.0
	for _, p := range pending {
		if p.rawScore > maxRaw {
			maxRaw = p.rawScore
		}
	}
	result := make([]scoredItem, 0, len(pending))
	for _, p := range pending {
		normF := 0.0
		if maxRaw > 0 {
			normF = p.rawScore / maxRaw
		}
		result = append(result, scoredItem{
			base:  p.base,
			score: combineScore(normF, m.normFrec[p.base.candidate.AbsPath]),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].score > result[j].score
	})
	return result
}

func scrollWindow(cursor, maxRows, total int) (start, end int) {
	if cursor >= maxRows {
		start = cursor - maxRows + 1
	}
	end = min(start+maxRows, total)
	return
}

func (m *model) rebuildFiltered() {
	query := m.tiQuery.Value()
	var result []scoredItem

	if query == "" {
		for _, item := range m.all {
			if m.switchOnly && !item.active {
				continue
			}
			if !m.matchesView(item) {
				continue
			}
			result = append(result, scoredItem{
				base:  item,
				score: m.normFrec[item.candidate.AbsPath],
			})
		}
		sort.SliceStable(result, func(i, j int) bool {
			return result[i].score > result[j].score
		})
	} else {
		// fuzzy match on RelPath — avoids ANSI escape codes in display strings
		keys := make([]string, len(m.all))
		for i, item := range m.all {
			keys[i] = item.candidate.RelPath
		}
		var pending []pendingItem
		for _, match := range fuzzy.Find(query, keys) {
			item := m.all[match.Index]
			if m.switchOnly && !item.active {
				continue
			}
			if !m.matchesView(item) {
				continue
			}
			pending = append(pending, pendingItem{base: item, rawScore: float64(match.Score)})
		}
		result = m.scoreAndSort(pending)
	}

	m.filtered = result
	if m.cursor >= len(m.filtered) {
		m.cursor = 0
	}
}

func (m *model) rebuildDestFiltered() {
	destQuery := m.tiDest.Value()

	// pool: non-repo candidates from m.all (projects + tmp projects)
	pool := make([]baseItem, 0, len(m.all))
	for _, item := range m.all {
		if !item.candidate.IsRepo {
			pool = append(pool, item)
		}
	}

	var result []scoredItem
	if destQuery == "" {
		for _, item := range pool {
			result = append(result, scoredItem{base: item, score: m.normFrec[item.candidate.AbsPath]})
		}
		sort.SliceStable(result, func(i, j int) bool { return result[i].score > result[j].score })
	} else {
		keys := make([]string, len(pool))
		for i, item := range pool {
			keys[i] = item.candidate.RelPath
		}
		var pending []pendingItem
		for _, match := range fuzzy.Find(destQuery, keys) {
			pending = append(pending, pendingItem{base: pool[match.Index], rawScore: float64(match.Score)})
		}
		result = m.scoreAndSort(pending)
	}
	m.destFiltered = result
	if m.destCursor >= len(m.destFiltered) {
		m.destCursor = 0
	}
}

func (m *model) rebuildCleanFiltered() {
	all := m.tmpItems()
	query := m.tiClean.Value()
	if query == "" {
		m.cleanFiltered = all
	} else {
		keys := make([]string, len(all))
		for i, item := range all {
			keys[i] = item.candidate.RelPath
		}
		matches := fuzzy.Find(query, keys)
		m.cleanFiltered = nil
		for _, match := range matches {
			m.cleanFiltered = append(m.cleanFiltered, all[match.Index])
		}
	}
	if m.cleanCursor >= len(m.cleanFiltered) {
		m.cleanCursor = 0
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
