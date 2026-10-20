package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
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

	entityID, err := sessionEntityID(tx, sessionName)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(
		"INSERT OR IGNORE INTO observations (entity_id, content, fact_type) VALUES (?, ?, ?)",
		entityID, summary, string(FactTypeSessionSummary),
	); err != nil {
		return fmt.Errorf("storing session summary: %w", err)
	}

	meta, err := readSessionMetadata(tx, entityID)
	if err != nil {
		return err
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

// sessionEntityID resolves a session's entity ID, returning ErrNotFound for unknown sessions.
func sessionEntityID(tx *sqlx.Tx, sessionName string) (int64, error) {
	var entityID int64
	err := tx.QueryRow(
		"SELECT id FROM entities WHERE name = ? AND is_latest = 1",
		sessionName,
	).Scan(&entityID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, fmt.Errorf("getting session entity: %w", err)
	}
	return entityID, nil
}

// readSessionMetadata reads and parses the session's container-tag metadata.
func readSessionMetadata(tx *sqlx.Tx, entityID int64) (SessionMetadata, error) {
	var tag string
	if err := tx.QueryRow(
		"SELECT COALESCE(container_tag, '') FROM entities WHERE id = ?",
		entityID,
	).Scan(&tag); err != nil {
		return SessionMetadata{}, fmt.Errorf("reading session metadata: %w", err)
	}

	var meta SessionMetadata
	if tag != "" {
		if err := json.Unmarshal([]byte(tag), &meta); err != nil {
			return SessionMetadata{}, fmt.Errorf("parsing session metadata: %w", err)
		}
	}
	return meta, nil
}

// sessionFromEntity converts an entity to a Session, reading project/status
// from the container-tag metadata.
func (s *Store) sessionFromEntity(entity *Entity) *Session {
	tag, _ := s.GetContainerTag(entity.Name)
	var meta SessionMetadata
	if tag != "" {
		_ = json.Unmarshal([]byte(tag), &meta)
	}
	return &Session{
		Name:      entity.Name,
		Project:   meta.Project,
		Status:    meta.Status,
		StartedAt: entity.CreatedAt,
	}
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
		session := s.sessionFromEntity(entity)
		if project != "" && session.Project != project {
			continue
		}
		if status != "" && session.Status != status {
			continue
		}
		sessions = append(sessions, session)
	}

	sortSessions(sessions)
	return limitSessions(sessions, limit), nil
}

// sortSessions orders sessions newest-first, breaking ties by name descending.
func sortSessions(sessions []*Session) {
	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].StartedAt.Equal(sessions[j].StartedAt) {
			return sessions[i].Name > sessions[j].Name
		}
		return sessions[i].StartedAt.After(sessions[j].StartedAt)
	})
}

// limitSessions truncates to limit entries (0 or negative = no limit).
func limitSessions(sessions []*Session, limit int) []*Session {
	if limit > 0 && len(sessions) > limit {
		return sessions[:limit]
	}
	return sessions
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

	return applyTokenBudget(s.filterSessionsByProject(results, project), tokenBudget), nil
}

// filterSessionsByProject keeps only sessions whose container-tag project
// matches. Empty project returns results unchanged.
func (s *Store) filterSessionsByProject(results []ContextResult, project string) []ContextResult {
	if project == "" {
		return results
	}
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
	return filtered
}

func (s *Store) GetSessionEvents(sessionName string) ([]SessionEvent, error) {
	entity, err := s.GetEntity(sessionName)
	if err != nil {
		return nil, err
	}
	if entity.Type != "session" {
		return nil, ErrNotFound
	}

	var contents []string
	if err := s.db.Select(&contents, `
		SELECT content FROM observations
		WHERE entity_id = ? AND fact_type = ? AND valid_until IS NULL
		ORDER BY created_at`, entity.ID, string(FactTypeSessionEvent)); err != nil {
		return nil, err
	}

	events := make([]SessionEvent, 0, len(contents))
	for _, c := range contents {
		var evt SessionEvent
		if err := json.Unmarshal([]byte(c), &evt); err == nil && evt.ToolName != "" {
			events = append(events, evt)
		}
	}
	return events, nil
}

func (s *Store) UpdateSessionSummary(sessionName, summary string) error {
	entity, err := s.GetEntity(sessionName)
	if err != nil {
		return err
	}
	if entity.Type != "session" {
		return ErrNotFound
	}

	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(
		"DELETE FROM observations WHERE entity_id = ? AND fact_type = ?",
		entity.ID, string(FactTypeSessionSummary),
	); err != nil {
		return fmt.Errorf("deleting existing summary: %w", err)
	}

	if _, err := tx.Exec(
		"INSERT INTO observations (entity_id, content, fact_type) VALUES (?, ?, ?)",
		entity.ID, summary, string(FactTypeSessionSummary),
	); err != nil {
		return fmt.Errorf("storing session summary: %w", err)
	}

	return tx.Commit()
}

func (s *Store) DeleteSessionEvents(sessionName string) error {
	entity, err := s.GetEntity(sessionName)
	if err != nil {
		return err
	}
	if entity.Type != "session" {
		return ErrNotFound
	}

	if _, err := s.db.Exec(
		"DELETE FROM observations WHERE entity_id = ? AND fact_type = ?",
		entity.ID, string(FactTypeSessionEvent),
	); err != nil {
		return fmt.Errorf("deleting session events: %w", err)
	}
	return nil
}

func (s *Store) GetSessionEventObservations(entityName string) ([]string, error) {
	entity, err := s.GetEntity(entityName)
	if err != nil {
		return nil, err
	}

	var contents []string
	if err := s.db.Select(&contents, `
		SELECT content FROM observations
		WHERE entity_id = ? AND fact_type = ? AND valid_until IS NULL
	`, entity.ID, string(FactTypeSessionEvent)); err != nil {
		return nil, err
	}
	return contents, nil
}
