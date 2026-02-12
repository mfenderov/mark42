package storage

import "strings"

// FactType represents the type of a fact/observation.
type FactType string

const (
	FactTypeStatic         FactType = "static"
	FactTypeDynamic        FactType = "dynamic"
	FactTypeSessionTurn    FactType = "session_turn"
	FactTypeSessionEvent   FactType = "session_event"
	FactTypeSessionSummary FactType = "session_summary"
)

// ObservationWithMeta represents an observation with metadata.
type ObservationWithMeta struct {
	EntityName string   `db:"entity_name"`
	EntityType string   `db:"entity_type"`
	Content    string   `db:"content"`
	FactType   FactType `db:"fact_type"`
}

// ContextByFactType holds observations grouped by fact type for context injection.
type ContextByFactType struct {
	Static      []ObservationWithMeta
	Dynamic     []ObservationWithMeta
	SessionTurn []ObservationWithMeta
}

// AddObservation adds an observation to an existing entity.
func (s *Store) AddObservation(entityName, content string) error {
	// Get entity ID
	var entityID int64
	err := s.db.QueryRow(
		"SELECT id FROM entities WHERE name = ?",
		entityName,
	).Scan(&entityID)
	if err != nil {
		return ErrNotFound
	}

	// Insert observation (ignore duplicate via INSERT OR IGNORE)
	_, err = s.db.Exec(
		"INSERT OR IGNORE INTO observations (entity_id, content) VALUES (?, ?)",
		entityID, content,
	)
	return err
}

// AddObservationWithType adds an observation with a specific fact type.
func (s *Store) AddObservationWithType(entityName, content string, factType FactType) error {
	var entityID int64
	err := s.db.QueryRow(
		"SELECT id FROM entities WHERE name = ?",
		entityName,
	).Scan(&entityID)
	if err != nil {
		return ErrNotFound
	}

	_, err = s.db.Exec(
		"INSERT OR IGNORE INTO observations (entity_id, content, fact_type) VALUES (?, ?, ?)",
		entityID, content, string(factType),
	)
	return err
}

// GetObservationsByFactType returns all observations of a specific fact type.
func (s *Store) GetObservationsByFactType(factType FactType) ([]ObservationWithMeta, error) {
	var results []ObservationWithMeta
	err := s.db.Select(&results, `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE o.fact_type = ?
		ORDER BY o.created_at DESC
	`, string(factType))
	return results, err
}

// GetContextByFactType returns all observations grouped by fact type for context injection.
func (s *Store) GetContextByFactType() (*ContextByFactType, error) {
	var observations []ObservationWithMeta
	err := s.db.Select(&observations, `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		ORDER BY
			CASE o.fact_type
				WHEN 'static' THEN 1
				WHEN 'dynamic' THEN 2
				WHEN 'session_turn' THEN 3
				ELSE 4
			END,
			o.created_at DESC
	`)
	if err != nil {
		return nil, err
	}

	// Group by fact type
	context := &ContextByFactType{}
	for _, obs := range observations {
		switch obs.FactType {
		case FactTypeStatic:
			context.Static = append(context.Static, obs)
		case FactTypeDynamic:
			context.Dynamic = append(context.Dynamic, obs)
		case FactTypeSessionTurn:
			context.SessionTurn = append(context.SessionTurn, obs)
		default:
			context.Dynamic = append(context.Dynamic, obs)
		}
	}
	return context, nil
}

// FormatContextForInjection formats observations by fact type for Claude context injection.
func (c *ContextByFactType) FormatContextForInjection() string {
	var sb strings.Builder

	sb.WriteString("<mark42-context>\n")

	if len(c.Static) > 0 {
		sb.WriteString("## User Profile (Persistent)\n")
		for _, obs := range c.Static {
			sb.WriteString("- ")
			sb.WriteString(obs.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(c.Dynamic) > 0 {
		sb.WriteString("## Recent Context\n")
		for _, obs := range c.Dynamic {
			sb.WriteString("- ")
			sb.WriteString(obs.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	if len(c.SessionTurn) > 0 {
		sb.WriteString("## Session History\n")
		for _, obs := range c.SessionTurn {
			sb.WriteString("- ")
			sb.WriteString(obs.Content)
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("</mark42-context>")
	return sb.String()
}

// GetObservationWithID looks up an observation by entity name and content, returning its ID.
// Returns nil if not found.
func (s *Store) GetObservationWithID(entityName, content string) *ObservationWithID {
	var obs ObservationWithID
	err := s.db.QueryRow(`
		SELECT o.id, o.content, e.name, e.entity_type
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.name = ? AND o.content = ?
	`, entityName, content).Scan(&obs.ID, &obs.Content, &obs.EntityName, &obs.EntityType)
	if err != nil {
		return nil
	}
	return &obs
}

// DeleteObservation removes a specific observation from an entity.
func (s *Store) DeleteObservation(entityName, content string) error {
	// Get entity ID
	var entityID int64
	err := s.db.QueryRow(
		"SELECT id FROM entities WHERE name = ?",
		entityName,
	).Scan(&entityID)
	if err != nil {
		return ErrNotFound
	}

	result, err := s.db.Exec(
		"DELETE FROM observations WHERE entity_id = ? AND content = ?",
		entityID, content,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rows == 0 {
		return ErrNotFound
	}

	return nil
}
