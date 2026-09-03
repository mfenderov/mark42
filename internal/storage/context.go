package storage

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/log"
)

var storageLogger = log.NewWithOptions(os.Stderr, log.Options{
	ReportTimestamp: false,
})

// ContextConfig holds configuration for context injection.
type ContextConfig struct {
	TokenBudget      int      // Maximum tokens to include (estimate: 4 chars = 1 token)
	MinImportance    float64  // Minimum importance score to include
	FactTypePriority []string // Priority order: static > dynamic > session_turn
	ProjectBoost     float64  // Score multiplier for project-matching memories
}

// DefaultContextConfig returns the default context injection configuration.
func DefaultContextConfig() ContextConfig {
	return ContextConfig{
		TokenBudget:      2000,
		MinImportance:    0.3,
		FactTypePriority: []string{"static", "dynamic", "session_turn"},
		ProjectBoost:     1.5,
	}
}

// ContextResult represents a memory selected for context injection.
type ContextResult struct {
	EntityName      string  `db:"entity_name"`
	EntityType      string  `db:"entity_type"`
	Content         string  `db:"content"`
	FactType        string  `db:"fact_type"`
	Importance      float64 `db:"importance"`
	DaysSinceAccess float64 `db:"days_since_access"`
	FinalScore      float64 // After fact type priority, project boost, and recency boost
}

var validFactTypes = map[string]bool{
	"static":          true,
	"dynamic":         true,
	"session_turn":    true,
	"session_summary": true,
}

// GetContextForInjection retrieves memories optimized for context injection.
// When query is non-empty, performs candidate search (FTS or hybrid) then re-ranks
// by importance×recency before applying token budget.
// When query is empty, uses flat importance ordering (original behavior).
func (s *Store) GetContextForInjection(cfg ContextConfig, projectName, query string, embedder Embedder) ([]ContextResult, error) {
	// Validate all fact types before string interpolation into ORDER BY clause
	for _, ft := range cfg.FactTypePriority {
		if !validFactTypes[ft] {
			return nil, fmt.Errorf("invalid fact type in FactTypePriority: %q", ft)
		}
	}

	if strings.TrimSpace(query) != "" {
		return s.getContextWithQuery(cfg, projectName, query, embedder)
	}

	return s.getContextFlat(cfg, projectName)
}

// getContextFlat is the original flat-importance path (query == "").
func (s *Store) getContextFlat(cfg ContextConfig, projectName string) ([]ContextResult, error) {
	// Build fact type priority case statement
	var factTypeCases []string
	for i, ft := range cfg.FactTypePriority {
		factTypeCases = append(factTypeCases, "WHEN '"+ft+"' THEN "+strconv.Itoa(i+1))
	}
	factTypeOrder := "CASE fact_type " + strings.Join(factTypeCases, " ") + " ELSE 99 END"

	// Query with ordering — includes days since last access for recency boost
	sqlQuery := `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type,
		       COALESCE(o.importance, 1.0) as importance,
		       COALESCE(julianday('now') - julianday(COALESCE(o.last_accessed, o.created_at)), 0) as days_since_access
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.importance >= ?
		AND COALESCE(o.fact_type, 'dynamic') != 'session_event'
		AND (o.valid_until IS NULL OR e.entity_type = 'session')
		ORDER BY ` + factTypeOrder + `, o.importance DESC
	`

	var results []ContextResult
	if err := s.db.Select(&results, sqlQuery, cfg.MinImportance); err != nil {
		return nil, err
	}

	applyScores(results, projectName, cfg.ProjectBoost)
	return applyTokenBudget(results, cfg.TokenBudget), nil
}

// getContextWithQuery performs candidate search then re-ranks by importance×recency.
func (s *Store) getContextWithQuery(cfg ContextConfig, projectName, query string, embedder Embedder) ([]ContextResult, error) {
	candidateLimit := cfg.TokenBudget * 3

	// Collect candidate entity names from search
	entityNames := make(map[string]struct{})

	if embedder != nil {
		ctx := context.Background()
		hybridResults, err := s.HybridSearchWithEmbedder(ctx, query, embedder, candidateLimit)
		if err != nil {
			storageLogger.Warn("hybrid search failed, falling back to FTS", "query", query, "error", err)
		} else {
			for _, r := range hybridResults {
				entityNames[r.EntityName] = struct{}{}
			}
		}
	}

	// FTS path (also used as fallback when embedder is nil or hybrid returned nothing)
	// TODO: SearchWithLimit does not filter by is_latest; superseded entity versions can consume
	// candidate slots. The second SQL (below) enforces is_latest=1, so results stay correct.
	if len(entityNames) == 0 {
		ftsResults, err := s.SearchWithLimit(query, candidateLimit)
		if err == nil {
			for _, r := range ftsResults {
				entityNames[r.Name] = struct{}{}
			}
		}
	}

	if len(entityNames) == 0 {
		return []ContextResult{}, nil
	}

	// Build IN clause for candidate entity names
	names := make([]string, 0, len(entityNames))
	for n := range entityNames {
		names = append(names, n)
	}

	placeholders := make([]string, len(names))
	args := make([]any, len(names)+1)
	args[0] = cfg.MinImportance
	for i, n := range names {
		placeholders[i] = "?"
		args[i+1] = n
	}

	inClause := strings.Join(placeholders, ", ")
	sqlQuery := `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type,
		       COALESCE(o.importance, 1.0) as importance,
		       COALESCE(julianday('now') - julianday(COALESCE(o.last_accessed, o.created_at)), 0) as days_since_access
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.importance >= ?
		AND e.name IN (` + inClause + `)
		AND COALESCE(o.fact_type, 'dynamic') != 'session_event'
		AND (o.valid_until IS NULL OR e.entity_type = 'session')
	`

	var results []ContextResult
	if err := s.db.Select(&results, sqlQuery, args...); err != nil {
		return nil, err
	}

	// Note: unlike getContextFlat, this path sorts by FinalScore only.
	// FactTypePriority SQL ordering is not applied; static facts get a 1.2× score boost instead.
	applyScores(results, projectName, cfg.ProjectBoost)

	// Sort by FinalScore descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	return applyTokenBudget(results, cfg.TokenBudget), nil
}

// applyScores calculates FinalScore for each result in place.
func applyScores(results []ContextResult, projectName string, projectBoost float64) {
	lowerProject := strings.ToLower(projectName)
	for i := range results {
		results[i].FinalScore = results[i].Importance

		// Recency boost: recently accessed memories get up to 1.5x, decays over ~30 days
		recencyBoost := 1.0 + 0.5*math.Exp(-results[i].DaysSinceAccess/30.0)
		results[i].FinalScore *= recencyBoost

		// Boost if entity or content matches project name
		if projectName != "" {
			if strings.Contains(strings.ToLower(results[i].EntityName), lowerProject) ||
				strings.Contains(strings.ToLower(results[i].Content), lowerProject) {
				results[i].FinalScore *= projectBoost
			}
		}

		// Boost static facts
		if results[i].FactType == "static" {
			results[i].FinalScore *= 1.2
		}
	}
}

// applyTokenBudget returns the subset of results that fit within the token budget.
func applyTokenBudget(results []ContextResult, tokenBudget int) []ContextResult {
	tokenCount := 0
	var selected []ContextResult
	for _, r := range results {
		entryTokens := (len(r.EntityName) + len(r.Content) + 20) / 4 // +20 for formatting
		if tokenCount+entryTokens > tokenBudget {
			break
		}
		tokenCount += entryTokens
		selected = append(selected, r)
	}
	return selected
}

// GetRecentContext retrieves memories ordered by recency, within the given time window.
// Prioritizes recently accessed observations, with optional project boosting.
func (s *Store) GetRecentContext(hours int, projectName string, tokenBudget int) ([]ContextResult, error) {
	if tokenBudget <= 0 {
		tokenBudget = 1000
	}

	query := `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type,
		       COALESCE(o.importance, 1.0) as importance,
		       COALESCE(julianday('now') - julianday(COALESCE(o.last_accessed, o.created_at)), 0) as days_since_access
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1
		AND COALESCE(o.last_accessed, o.created_at) > datetime('now', ? || ' hours')
		AND COALESCE(o.fact_type, 'dynamic') != 'session_event'
		AND (o.valid_until IS NULL OR e.entity_type = 'session')
		ORDER BY COALESCE(o.last_accessed, o.created_at) DESC
	`

	hoursParam := "-" + strconv.Itoa(hours)

	var results []ContextResult
	if err := s.db.Select(&results, query, hoursParam); err != nil {
		return nil, err
	}

	// Apply project boost
	for i := range results {
		results[i].FinalScore = results[i].Importance
		if projectName != "" {
			lowerProject := strings.ToLower(projectName)
			if strings.Contains(strings.ToLower(results[i].EntityName), lowerProject) ||
				strings.Contains(strings.ToLower(results[i].Content), lowerProject) {
				results[i].FinalScore *= 1.5
			}
		}
	}

	// Apply token budget
	tokenCount := 0
	var selected []ContextResult
	for _, r := range results {
		entryTokens := (len(r.EntityName) + len(r.Content) + 20) / 4
		if tokenCount+entryTokens > tokenBudget {
			break
		}
		tokenCount += entryTokens
		selected = append(selected, r)
	}

	return selected, nil
}

// FormatContextResults formats context results for injection into conversation.
func FormatContextResults(results []ContextResult) string {
	if len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== Relevant Memories ===\n\n")

	// Group by fact type
	staticObs := make(map[string][]string)
	dynamicObs := make(map[string][]string)
	sessionObs := make(map[string][]string)

	for _, r := range results {
		key := r.EntityName + " (" + r.EntityType + ")"
		switch r.FactType {
		case "static":
			staticObs[key] = append(staticObs[key], r.Content)
		case "session_turn":
			sessionObs[key] = append(sessionObs[key], r.Content)
		default:
			dynamicObs[key] = append(dynamicObs[key], r.Content)
		}
	}

	// Output static first (user preferences)
	if len(staticObs) > 0 {
		sb.WriteString("[STATIC] Project Conventions:\n")
		for entity, observations := range staticObs {
			sb.WriteString("## " + entity + "\n")
			for _, obs := range observations {
				sb.WriteString("- " + obs + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// Output dynamic (recent context)
	if len(dynamicObs) > 0 {
		sb.WriteString("[DYNAMIC] Recent Context:\n")
		for entity, observations := range dynamicObs {
			sb.WriteString("## " + entity + "\n")
			for _, obs := range observations {
				sb.WriteString("- " + obs + "\n")
			}
		}
		sb.WriteString("\n")
	}

	// Output session turns (conversation history)
	if len(sessionObs) > 0 {
		sb.WriteString("[SESSION] Conversation History:\n")
		for entity, observations := range sessionObs {
			sb.WriteString("## " + entity + "\n")
			for _, obs := range observations {
				sb.WriteString("- " + obs + "\n")
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// EstimateTokens estimates the number of tokens in the context.
// Uses simple heuristic: 4 characters ≈ 1 token.
func EstimateTokens(text string) int {
	return len(text) / 4
}

// FormatSessionRecall formats session summaries for recall injection.
func FormatSessionRecall(results []ContextResult) string {
	if len(results) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("=== Recent Sessions ===\n\n")

	for _, r := range results {
		sb.WriteString("- [" + r.EntityName + "] " + r.Content + "\n")
	}

	return sb.String()
}
