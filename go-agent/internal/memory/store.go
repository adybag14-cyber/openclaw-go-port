package memory

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type MessageEntry struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionId,omitempty"`
	Channel   string         `json:"channel,omitempty"`
	Method    string         `json:"method"`
	Role      string         `json:"role"`
	Text      string         `json:"text,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
	CreatedAt string         `json:"createdAt"`
}

type GraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Weight int    `json:"weight"`
}

type Store struct {
	mu         sync.RWMutex
	path       string
	persist    bool
	maxEntries int
	entries    []MessageEntry
	vectors    map[string]map[string]float64
	graph      map[string]map[string]int
	lastError  string
}

type persistedState struct {
	Entries []MessageEntry                `json:"entries"`
	Vectors map[string]map[string]float64 `json:"vectors,omitempty"`
	Graph   map[string]map[string]int     `json:"graph,omitempty"`
}

func NewStore(path string, maxEntries int) *Store {
	switch {
	case maxEntries <= 0:
		// 0/-1/negative values are treated as unlimited retention.
		maxEntries = 0
	case maxEntries < 100:
		maxEntries = 100
	}
	initialCapacity := maxEntries
	if initialCapacity <= 0 {
		initialCapacity = 1024
	}
	s := &Store{
		path:       strings.TrimSpace(path),
		persist:    shouldPersist(path),
		maxEntries: maxEntries,
		entries:    make([]MessageEntry, 0, initialCapacity),
		vectors:    map[string]map[string]float64{},
		graph:      map[string]map[string]int{},
	}
	s.load()
	return s
}

func (s *Store) Append(entry MessageEntry) {
	s.mu.Lock()
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if entry.ID == "" {
		entry.ID = "msg-" + time.Now().UTC().Format("20060102150405.000000000")
	}
	s.entries = append(s.entries, entry)
	if s.maxEntries > 0 && len(s.entries) > s.maxEntries {
		start := len(s.entries) - s.maxEntries
		s.entries = append([]MessageEntry(nil), s.entries[start:]...)
	}
	s.rebuildIndexesLocked()
	s.mu.Unlock()
	s.persistLockedSnapshot()
}

func (s *Store) HistoryBySession(sessionID string, limit int) []MessageEntry {
	if limit <= 0 {
		limit = 50
	}
	sid := strings.TrimSpace(sessionID)
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MessageEntry, 0, limit)
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if sid != "" && entry.SessionID != sid {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	reverse(out)
	return out
}

func (s *Store) HistoryByChannel(channel string, limit int) []MessageEntry {
	if limit <= 0 {
		limit = 50
	}
	canonical := strings.ToLower(strings.TrimSpace(channel))
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MessageEntry, 0, limit)
	for i := len(s.entries) - 1; i >= 0; i-- {
		entry := s.entries[i]
		if canonical != "" && strings.ToLower(entry.Channel) != canonical {
			continue
		}
		out = append(out, entry)
		if len(out) >= limit {
			break
		}
	}
	reverse(out)
	return out
}

func (s *Store) SemanticRecall(query string, limit int) []MessageEntry {
	if limit <= 0 {
		limit = 5
	}
	queryVector := embedText(query)
	if len(queryVector) == 0 {
		return []MessageEntry{}
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	type scored struct {
		entry MessageEntry
		score float64
	}
	scoredEntries := make([]scored, 0, len(s.entries))
	for _, entry := range s.entries {
		vec, ok := s.vectors[entry.ID]
		if !ok || len(vec) == 0 {
			continue
		}
		score := cosineSimilarity(queryVector, vec)
		if score <= 0 {
			continue
		}
		scoredEntries = append(scoredEntries, scored{entry: entry, score: score})
	}
	sort.Slice(scoredEntries, func(i, j int) bool {
		return scoredEntries[i].score > scoredEntries[j].score
	})
	if len(scoredEntries) > limit {
		scoredEntries = scoredEntries[:limit]
	}
	out := make([]MessageEntry, 0, len(scoredEntries))
	for _, item := range scoredEntries {
		out = append(out, item.entry)
	}
	return out
}

func (s *Store) GraphNeighbors(node string, limit int) []GraphEdge {
	if limit <= 0 {
		limit = 10
	}
	key := strings.ToLower(strings.TrimSpace(node))
	if key == "" {
		return []GraphEdge{}
	}

	s.mu.RLock()
	adj := s.graph[key]
	s.mu.RUnlock()
	if len(adj) == 0 {
		return []GraphEdge{}
	}

	edges := make([]GraphEdge, 0, len(adj))
	for to, weight := range adj {
		edges = append(edges, GraphEdge{
			From:   key,
			To:     to,
			Weight: weight,
		})
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Weight == edges[j].Weight {
			return edges[i].To < edges[j].To
		}
		return edges[i].Weight > edges[j].Weight
	})
	if len(edges) > limit {
		edges = edges[:limit]
	}
	return edges
}

func (s *Store) RecallSynthesis(query string, limit int) map[string]any {
	semantic := s.SemanticRecall(query, limit)
	term := firstSignificantTerm(query)
	neighbors := []GraphEdge{}
	if term != "" {
		neighbors = s.GraphNeighbors("term:"+term, limit)
	}
	return map[string]any{
		"query":     strings.TrimSpace(query),
		"semantic":  semantic,
		"neighbors": neighbors,
		"count": map[string]any{
			"semantic":  len(semantic),
			"neighbors": len(neighbors),
		},
	}
}

func (s *Store) Stats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	graphEdges := 0
	for _, adj := range s.graph {
		graphEdges += len(adj)
	}
	return map[string]any{
		"entries":    len(s.entries),
		"vectors":    len(s.vectors),
		"graphNodes": len(s.graph),
		"graphEdges": graphEdges,
		"maxEntries": s.maxEntries,
		"unlimited":  s.maxEntries <= 0,
		"persistent": s.persist,
		"statePath":  s.path,
		"lastError":  s.lastError,
	}
}

func (s *Store) LastError() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastError
}

func (s *Store) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.entries)
}

func (s *Store) Trim(limit int) int {
	if limit < 0 {
		limit = 0
	}

	removed := 0
	s.mu.Lock()
	switch {
	case limit == 0 && len(s.entries) > 0:
		removed = len(s.entries)
		s.entries = []MessageEntry{}
	case limit > 0 && len(s.entries) > limit:
		removed = len(s.entries) - limit
		start := len(s.entries) - limit
		s.entries = append([]MessageEntry(nil), s.entries[start:]...)
	}
	if removed > 0 {
		s.rebuildIndexesLocked()
	}
	s.mu.Unlock()

	if removed > 0 {
		s.persistLockedSnapshot()
	}
	return removed
}

func (s *Store) RemoveSession(sessionID string) int {
	sid := strings.TrimSpace(sessionID)
	if sid == "" {
		return 0
	}

	removed := 0
	s.mu.Lock()
	if len(s.entries) > 0 {
		kept := make([]MessageEntry, 0, len(s.entries))
		for _, entry := range s.entries {
			if strings.TrimSpace(entry.SessionID) == sid {
				removed++
				continue
			}
			kept = append(kept, entry)
		}
		if removed > 0 {
			s.entries = append([]MessageEntry(nil), kept...)
			s.rebuildIndexesLocked()
		}
	}
	s.mu.Unlock()

	if removed > 0 {
		s.persistLockedSnapshot()
	}
	return removed
}

func (s *Store) load() {
	if !s.persist {
		return
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return
		}
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	var data persistedState
	if err := json.Unmarshal(raw, &data); err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	s.mu.Lock()
	s.entries = append([]MessageEntry(nil), data.Entries...)
	if s.maxEntries > 0 && len(s.entries) > s.maxEntries {
		start := len(s.entries) - s.maxEntries
		s.entries = append([]MessageEntry(nil), s.entries[start:]...)
	}
	s.rebuildIndexesLocked()
	s.mu.Unlock()
}

func (s *Store) persistLockedSnapshot() {
	if !s.persist {
		return
	}
	s.mu.RLock()
	payload := persistedState{
		Entries: append([]MessageEntry(nil), s.entries...),
		Vectors: cloneVectors(s.vectors),
		Graph:   cloneGraph(s.graph),
	}
	path := s.path
	s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		s.mu.Lock()
		s.lastError = err.Error()
		s.mu.Unlock()
		return
	}
}

func (s *Store) rebuildIndexesLocked() {
	s.vectors = map[string]map[string]float64{}
	s.graph = map[string]map[string]int{}
	for _, entry := range s.entries {
		if vector := embedText(entry.Text); len(vector) > 0 {
			s.vectors[entry.ID] = vector
		}
		term := firstSignificantTerm(entry.Text)
		if term != "" {
			s.addGraphEdgeLocked("entry:"+entry.ID, "term:"+term)
		}
		if sid := strings.TrimSpace(entry.SessionID); sid != "" {
			s.addGraphEdgeLocked("session:"+sid, "entry:"+entry.ID)
			if channel := strings.ToLower(strings.TrimSpace(entry.Channel)); channel != "" {
				s.addGraphEdgeLocked("session:"+sid, "channel:"+channel)
			}
		}
		if channel := strings.ToLower(strings.TrimSpace(entry.Channel)); channel != "" {
			s.addGraphEdgeLocked("channel:"+channel, "method:"+strings.ToLower(strings.TrimSpace(entry.Method)))
		}
		role := strings.ToLower(strings.TrimSpace(entry.Role))
		if role != "" {
			s.addGraphEdgeLocked("role:"+role, "method:"+strings.ToLower(strings.TrimSpace(entry.Method)))
		}
	}
}

func (s *Store) addGraphEdgeLocked(from string, to string) {
	fromKey := strings.ToLower(strings.TrimSpace(from))
	toKey := strings.ToLower(strings.TrimSpace(to))
	if fromKey == "" || toKey == "" {
		return
	}
	if _, ok := s.graph[fromKey]; !ok {
		s.graph[fromKey] = map[string]int{}
	}
	s.graph[fromKey][toKey]++
}

func shouldPersist(path string) bool {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(trimmed), "memory://") {
		return false
	}
	return true
}

func embedText(text string) map[string]float64 {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	if len(tokens) == 0 {
		return map[string]float64{}
	}
	freq := map[string]float64{}
	for _, token := range tokens {
		token = strings.Trim(token, ".,!?;:\"'()[]{}")
		if len(token) < 2 {
			continue
		}
		freq[token]++
	}
	if len(freq) == 0 {
		return map[string]float64{}
	}
	var norm float64
	for _, value := range freq {
		norm += value * value
	}
	norm = math.Sqrt(norm)
	if norm == 0 {
		return freq
	}
	for token, value := range freq {
		freq[token] = value / norm
	}
	return freq
}

func cosineSimilarity(a map[string]float64, b map[string]float64) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	dot := 0.0
	for token, av := range a {
		if bv, ok := b[token]; ok {
			dot += av * bv
		}
	}
	return dot
}

func firstSignificantTerm(text string) string {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	for _, token := range tokens {
		token = strings.Trim(token, ".,!?;:\"'()[]{}")
		if len(token) >= 4 {
			return token
		}
	}
	return ""
}

func cloneVectors(input map[string]map[string]float64) map[string]map[string]float64 {
	out := make(map[string]map[string]float64, len(input))
	for key, vector := range input {
		row := make(map[string]float64, len(vector))
		for term, value := range vector {
			row[term] = value
		}
		out[key] = row
	}
	return out
}

func cloneGraph(input map[string]map[string]int) map[string]map[string]int {
	out := make(map[string]map[string]int, len(input))
	for from, adj := range input {
		row := make(map[string]int, len(adj))
		for to, weight := range adj {
			row[to] = weight
		}
		out[from] = row
	}
	return out
}

func reverse[T any](items []T) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}
