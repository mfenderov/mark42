package storage

import (
	"math"
	"strings"
)

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

// GetContextForInjection retrieves memories optimized for context injection.
// Orders by: fact type priority, then importance, respecting token budget.
func (s *Store) GetContextForInjection(cfg ContextConfig, projectName string) ([]ContextResult, error) {
	// Build fact type priority case statement
	var factTypeCases []string
	for i, ft := range cfg.FactTypePriority {
		factTypeCases = append(factTypeCases, "WHEN '"+ft+"' THEN "+formatInt(i+1))
	}
	factTypeOrder := "CASE fact_type " + strings.Join(factTypeCases, " ") + " ELSE 99 END"

	// Query with ordering — includes days since last access for recency boost
	query := `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type,
		       COALESCE(o.importance, 1.0) as importance,
		       COALESCE(julianday('now') - julianday(COALESCE(o.last_accessed, o.created_at)), 0) as days_since_access
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.importance >= ?
		ORDER BY ` + factTypeOrder + `, o.importance DESC
	`

	var results []ContextResult
	err := s.db.Select(&results, query, cfg.MinImportance)
	if err != nil {
		return nil, err
	}

	// Apply boosts and calculate final scores:
	// final_score = importance × recency_boost × project_boost × fact_type_boost
	for i := range results {
		results[i].FinalScore = results[i].Importance

		// Recency boost: recently accessed memories get up to 1.5x, decays over ~30 days
		recencyBoost := 1.0 + 0.5*math.Exp(-results[i].DaysSinceAccess/30.0)
		results[i].FinalScore *= recencyBoost

		// Boost if entity or content matches project name
		if projectName != "" {
			lowerProject := strings.ToLower(projectName)
			if strings.Contains(strings.ToLower(results[i].EntityName), lowerProject) ||
				strings.Contains(strings.ToLower(results[i].Content), lowerProject) {
				results[i].FinalScore *= cfg.ProjectBoost
			}
		}

		// Boost static facts
		if results[i].FactType == "static" {
			results[i].FinalScore *= 1.2
		}
	}

	// Apply token budget (estimate 4 chars per token)
	tokenCount := 0
	var selected []ContextResult
	for _, r := range results {
		// Estimate tokens for this entry
		entryTokens := (len(r.EntityName) + len(r.Content) + 20) / 4 // +20 for formatting
		if tokenCount+entryTokens > cfg.TokenBudget {
			break
		}
		tokenCount += entryTokens
		selected = append(selected, r)
	}

	return selected, nil
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
		ORDER BY COALESCE(o.last_accessed, o.created_at) DESC
	`

	hoursParam := "-" + formatInt(hours)

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

// formatInt converts int to string without importing fmt.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + formatInt(-n)
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
