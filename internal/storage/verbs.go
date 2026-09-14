package storage

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// RememberParams defines the parameters for storing or updating memories.
type RememberParams struct {
	Topic      string
	Facts      []string
	EntityType string
	FactType   FactType
	Relations  []RelationParam
	Project    string
}

// RelationParam defines a target topic and relationship type.
type RelationParam struct {
	To   string
	Type string
}

// RememberResult summarizes the result of a Remember operation.
type RememberResult struct {
	Topic             string
	ObservationsAdded int
	RelationsCreated  int
}

// RecallParams defines the criteria for memory retrieval.
type RecallParams struct {
	Query       string
	Topic       string
	Project     string
	Limit       int
	TokenBudget int
}

// RecallResult contains retrieved memories formatted by retrieval type.
type RecallResult struct {
	Type          string          // "topic", "search", or "context"
	Topic         *Entity         // Populated when Type == "topic"
	Relations     []*Relation     // Populated when Type == "topic"
	SearchResults []FusedResult   // Populated when Type == "search"
	Context       []ContextResult // Populated when Type == "context"
	Sessions      []ContextResult // Populated when Type == "context"
}

// ForgetParams defines the parameters for forgetting knowledge.
type ForgetParams struct {
	Topic     string
	Fact      string
	Permanent bool
}

// ForgetResult summarizes the result of a Forget operation.
type ForgetResult struct {
	Topic     string
	Count     int
	Permanent bool
}

// Remember stores or updates knowledge in memory under a topic.
func (s *Store) Remember(ctx context.Context, params RememberParams) (*RememberResult, error) {
	if strings.TrimSpace(params.Topic) == "" {
		return nil, fmt.Errorf("topic cannot be empty")
	}

	entityType := params.EntityType
	if strings.TrimSpace(entityType) == "" {
		entityType = "concept"
	}

	factType := params.FactType
	if factType == "" {
		factType = FactTypeStatic
	}

	// 1. Ensure entity exists
	if err := s.ensureTopicEntity(params.Topic, entityType, params.EntityType, params.Project); err != nil {
		return nil, err
	}

	// 2. Add observations
	addedFacts, addedCount, err := s.insertFacts(params.Topic, params.Facts, factType)
	if err != nil {
		return nil, err
	}

	// 3. Auto-embed observations on write if embedder is available
	s.embedAndExpire(ctx, params.Topic, addedFacts, factType)

	// 4. Create relations
	relationsCreated := s.linkRelations(params.Topic, params.Project, params.Relations)

	return &RememberResult{
		Topic:             params.Topic,
		ObservationsAdded: addedCount,
		RelationsCreated:  relationsCreated,
	}, nil
}

func (s *Store) ensureTopicEntity(topic, entityType, explicitType, project string) error {
	existing, err := s.GetEntity(topic)
	if err == ErrNotFound {
		if project != "" {
			_, err = s.CreateEntityWithContainer(topic, entityType, nil, project)
		} else {
			_, err = s.CreateEntity(topic, entityType, nil)
		}
		if err != nil {
			return fmt.Errorf("failed to create entity %q: %w", topic, err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to inspect entity %q: %w", topic, err)
	}
	if explicitType != "" && existing.Type != explicitType {
		_, _ = s.db.Exec("UPDATE entities SET entity_type = ? WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL)", explicitType, topic)
	}
	return nil
}

func (s *Store) insertFacts(topic string, facts []string, factType FactType) ([]string, int, error) {
	addedCount := 0
	var addedFacts []string
	for _, fact := range facts {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if err := s.AddObservationWithType(topic, fact, factType); err != nil {
			return nil, 0, fmt.Errorf("failed to add observation to %q: %w", topic, err)
		}
		addedCount++
		addedFacts = append(addedFacts, fact)
	}
	return addedFacts, addedCount, nil
}

func (s *Store) embedAndExpire(ctx context.Context, topic string, addedFacts []string, factType FactType) {
	if s.embedder == nil || len(addedFacts) == 0 {
		return
	}
	s.embedFacts(ctx, topic, addedFacts)
	s.expireSuperseded(topic, addedFacts, factType)
}

func (s *Store) embedFacts(ctx context.Context, topic string, addedFacts []string) {
	embedCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	for _, fact := range addedFacts {
		if embedCtx.Err() != nil {
			break
		}
		embedding, err := s.embedder.CreateEmbedding(embedCtx, fact)
		if err != nil {
			storageLogger.Warn("auto-embed failed in Remember", "topic", topic, "error", err)
			continue
		}
		obs := s.GetObservationWithID(topic, fact)
		if obs != nil {
			_ = s.StoreEmbedding(obs.ID, embedding, "nomic-embed-text")
		}
	}
}

func (s *Store) expireSuperseded(topic string, addedFacts []string, factType FactType) {
	if factType == FactTypeSessionTurn || factType == FactTypeSessionEvent || factType == FactTypeSessionSummary {
		return
	}
	_, _ = s.DetectAndExpireSupersededBatch(topic, addedFacts, s.embedder, DefaultSupersessionThreshold)
}

func (s *Store) linkRelations(from, project string, rels []RelationParam) int {
	relationsCreated := 0
	for _, rel := range rels {
		to := strings.TrimSpace(rel.To)
		relType := strings.TrimSpace(rel.Type)
		if to == "" || relType == "" {
			continue
		}

		_, err := s.GetEntity(to)
		if err == ErrNotFound {
			if project != "" {
				_, _ = s.CreateEntityWithContainer(to, "concept", nil, project)
			} else {
				_, _ = s.CreateEntity(to, "concept", nil)
			}
		}

		if err := s.CreateRelation(from, to, relType); err == nil {
			relationsCreated++
		}
	}
	return relationsCreated
}

// Recall retrieves memories matching query, topic, or default project context.
func (s *Store) Recall(ctx context.Context, params RecallParams) (*RecallResult, error) {
	if params.Topic != "" {
		return s.recallTopic(params.Topic)
	}

	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}

	if strings.TrimSpace(params.Query) != "" {
		return s.recallSearch(ctx, params.Query, limit)
	}

	return s.recallContext(params.Project, params.TokenBudget)
}

func (s *Store) recallTopic(topic string) (*RecallResult, error) {
	entity, err := s.GetEntity(topic)
	if err != nil {
		return nil, err
	}

	relations, err := s.ListRelations(topic)
	if err != nil {
		relations = []*Relation{}
	}

	return &RecallResult{
		Type:      "topic",
		Topic:     entity,
		Relations: relations,
	}, nil
}

func (s *Store) recallSearch(ctx context.Context, query string, limit int) (*RecallResult, error) {
	var queryEmbedding []float64
	if s.embedder != nil {
		vec, err := s.embedder.CreateEmbedding(ctx, query)
		if err == nil {
			queryEmbedding = vec
		}
	}

	fused, err := s.HybridSearch(ctx, query, queryEmbedding, limit)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	for _, item := range fused {
		s.touchObservationAccess(item.EntityName, item.Content)
	}

	return &RecallResult{
		Type:          "search",
		SearchResults: fused,
	}, nil
}

func (s *Store) recallContext(project string, tokenBudget int) (*RecallResult, error) {
	if tokenBudget <= 0 {
		tokenBudget = 2000
	}

	cfg := DefaultContextConfig()
	cfg.TokenBudget = tokenBudget

	contextResults, err := s.GetContextForInjection(cfg, project, "", s.embedder)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve context: %w", err)
	}

	sessionResults, _ := s.GetRecentSessionSummaries(project, 72, 1000)

	return &RecallResult{
		Type:     "context",
		Context:  contextResults,
		Sessions: sessionResults,
	}, nil
}

// Forget invalidates or permanently deletes memories.
func (s *Store) Forget(ctx context.Context, params ForgetParams) (*ForgetResult, error) {
	if strings.TrimSpace(params.Topic) == "" {
		return nil, fmt.Errorf("topic cannot be empty")
	}

	if strings.TrimSpace(params.Fact) != "" {
		return s.forgetFact(params.Topic, params.Fact, params.Permanent)
	}

	return s.forgetTopic(params.Topic, params.Permanent)
}

func (s *Store) forgetFact(topic, fact string, permanent bool) (*ForgetResult, error) {
	if permanent {
		if err := s.DeleteObservation(topic, fact); err != nil {
			return nil, err
		}
		return &ForgetResult{
			Topic:     topic,
			Count:     1,
			Permanent: true,
		}, nil
	}

	if err := s.InvalidateObservation(topic, fact); err != nil {
		return nil, err
	}
	return &ForgetResult{
		Topic:     topic,
		Count:     1,
		Permanent: false,
	}, nil
}

func (s *Store) forgetTopic(topic string, permanent bool) (*ForgetResult, error) {
	if permanent {
		if err := s.DeleteEntity(topic); err != nil {
			return nil, err
		}
		return &ForgetResult{
			Topic:     topic,
			Count:     1,
			Permanent: true,
		}, nil
	}

	res, err := s.db.Exec(`
		UPDATE observations
		SET valid_until = CURRENT_TIMESTAMP
		WHERE entity_id = (SELECT id FROM entities WHERE name = ? AND (is_latest = 1 OR is_latest IS NULL))
		  AND valid_until IS NULL
	`, topic)
	if err != nil {
		return nil, err
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}

	return &ForgetResult{
		Topic:     topic,
		Count:     int(rows),
		Permanent: false,
	}, nil
}

// touchObservationAccess records access timestamp and increments access count.
func (s *Store) touchObservationAccess(entityName, content string) {
	_, _ = s.db.Exec(`
		UPDATE observations
		SET access_count = COALESCE(access_count, 0) + 1,
		    last_accessed = CURRENT_TIMESTAMP
		WHERE entity_id = (SELECT id FROM entities WHERE name = ?)
		  AND content = ?
	`, entityName, content)
}
