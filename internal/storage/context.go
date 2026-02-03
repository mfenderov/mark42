package storage

import (
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
	EntityName  string  `db:"entity_name"`
	EntityType  string  `db:"entity_type"`
	Content     string  `db:"content"`
	FactType    string  `db:"fact_type"`
	Importance  float64 `db:"importance"`
	FinalScore  float64 // After fact type priority and project boost
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

	// Query with ordering
	query := `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type,
		       COALESCE(o.importance, 1.0) as importance
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

	// Apply project boost and calculate final scores
	for i := range results {
		results[i].FinalScore = results[i].Importance

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
