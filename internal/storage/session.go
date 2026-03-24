package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Session struct {
	Name       string
	Project    string
	Status     string
	Summary    string
	EventCount int
	StartedAt  time.Time
	EndedAt    time.Time
}

type SessionEvent struct {
	ToolName  string `json:"toolName"`
	FilePath  string `json:"filePath,omitempty"`
	Command   string `json:"command,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
}

type SessionMetadata struct {
	Project   string `json:"project"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
	EndedAt   string `json:"endedAt,omitempty"`
}

func (s *Store) CreateSession(project string) (*Session, error) {
	now := time.Now()
	name := fmt.Sprintf("session-%s-%s-%d", project, now.Format("20060102-150405"), now.UnixNano()%1e9)

	meta := SessionMetadata{
		Project:   project,
		Status:    "active",
		StartedAt: now.Format(time.RFC3339),
	}
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session metadata: %w", err)
	}

	_, err = s.CreateEntity(name, "session", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create session entity: %w", err)
	}

	if err := s.SetContainerTag(name, string(metaJSON)); err != nil {
		return nil, fmt.Errorf("failed to set session metadata: %w", err)
	}

	return &Session{
		Name:      name,
		Project:   project,
		Status:    "active",
		StartedAt: now,
	}, nil
}

func (s *Store) CaptureSessionEvent(sessionName string, event SessionEvent) error {
	content, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	return s.AddObservationWithType(sessionName, string(content), FactTypeSessionEvent)
}

func (s *Store) CompleteSession(sessionName, summary string) error {
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	var entityID int64
	if err := tx.QueryRow(
		"SELECT id FROM entities WHERE name = ? AND is_latest = 1",
		sessionName,
	).Scan(&entityID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("getting session entity: %w", err)
	}

	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO observations (entity_id, content, fact_type) VALUES (?, ?, ?)",
		entityID, summary, string(FactTypeSessionSummary),
	); err != nil {
		return fmt.Errorf("storing session summary: %w", err)
	}

	var tag string
	if err := tx.QueryRow(
		"SELECT COALESCE(container_tag, '') FROM entities WHERE id = ?",
		entityID,
	).Scan(&tag); err != nil {
		return fmt.Errorf("reading session metadata: %w", err)
	}

	var meta SessionMetadata
	if tag != "" {
		if err := json.Unmarshal([]byte(tag), &meta); err != nil {
			return fmt.Errorf("parsing session metadata: %w", err)
		}
	}
	meta.Status = "completed"
	meta.EndedAt = time.Now().Format(time.RFC3339)

	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshaling session metadata: %w", err)
	}

	if _, err := tx.Exec(
		"UPDATE entities SET container_tag = ? WHERE id = ?",
		string(metaJSON), entityID,
	); err != nil {
		return fmt.Errorf("updating session metadata: %w", err)
	}

	return tx.Commit()
}

func (s *Store) GetSession(sessionName string) (*Session, error) {
	entity, err := s.GetEntity(sessionName)
	if err != nil {
		return nil, err
	}
	if entity.Type != "session" {
		return nil, ErrNotFound
	}

	tag, _ := s.GetContainerTag(sessionName)
	var meta SessionMetadata
	if tag != "" {
		_ = json.Unmarshal([]byte(tag), &meta)
	}

	// Count events and find summary
	var summary string
	var eventCount int
	for _, obs := range entity.Observations {
		// Try to parse as event JSON
		var evt SessionEvent
		if err := json.Unmarshal([]byte(obs), &evt); err == nil && evt.ToolName != "" {
			eventCount++
		} else {
			summary = obs
		}
	}

	session := &Session{
		Name:       entity.Name,
		Project:    meta.Project,
		Status:     meta.Status,
		Summary:    summary,
		EventCount: eventCount,
		StartedAt:  entity.CreatedAt,
	}

	if meta.EndedAt != "" {
		session.EndedAt, _ = time.Parse(time.RFC3339, meta.EndedAt)
	}

	return session, nil
}

func (s *Store) ListSessions(project, status string, limit int) ([]*Session, error) {
	entities, err := s.ListEntities("session")
	if err != nil {
		return nil, err
	}

	var sessions []*Session
	for _, entity := range entities {
		tag, _ := s.GetContainerTag(entity.Name)
		var meta SessionMetadata
		if tag != "" {
			_ = json.Unmarshal([]byte(tag), &meta)
		}

		if project != "" && meta.Project != project {
			continue
		}
		if status != "" && meta.Status != status {
			continue
		}

		sessions = append(sessions, &Session{
			Name:      entity.Name,
			Project:   meta.Project,
			Status:    meta.Status,
			StartedAt: entity.CreatedAt,
		})

		if len(sessions) >= limit {
			break
		}
	}

	return sessions, nil
}

func (s *Store) GetRecentSessionSummaries(project string, hours, tokenBudget int) ([]ContextResult, error) {
	if hours <= 0 {
		hours = 72
	}
	if tokenBudget <= 0 {
		tokenBudget = 1500
	}

	hoursParam := "-" + strconv.Itoa(hours)

	query := `
		SELECT e.name as entity_name, e.entity_type, o.content,
		       COALESCE(o.fact_type, 'dynamic') as fact_type,
		       COALESCE(o.importance, 1.0) as importance,
		       COALESCE(julianday('now') - julianday(COALESCE(o.last_accessed, o.created_at)), 0) as days_since_access
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.entity_type = 'session'
		AND o.fact_type = 'session_summary'
		AND COALESCE(o.last_accessed, o.created_at) > datetime('now', ? || ' hours')
		ORDER BY o.created_at DESC
	`

	var results []ContextResult
	if err := s.db.Select(&results, query, hoursParam); err != nil {
		return nil, err
	}

	// Filter by project if specified
	if project != "" {
		var filtered []ContextResult
		for _, r := range results {
			tag, _ := s.GetContainerTag(r.EntityName)
			var meta SessionMetadata
			if tag != "" {
				_ = json.Unmarshal([]byte(tag), &meta)
			}
			if meta.Project == project {
				filtered = append(filtered, r)
			}
		}
		results = filtered
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
