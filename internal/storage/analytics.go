package storage

import "os"

// AgeBucket represents a decay-curve bucket: how many valid observations
// fall into an age range, and their average importance.
type AgeBucket struct {
	Label         string  // "0-7d", "7-30d", "30-90d", "90d+"
	Count         int     `db:"count"`
	AvgImportance float64 `db:"avg_importance"`
}

// AccessedObservation is a single observation ranked by access_count.
type AccessedObservation struct {
	EntityName  string `db:"entity_name"`
	Content     string `db:"content"`
	AccessCount int    `db:"access_count"`
}

// WeeklyCount is the number of sessions captured in a given ISO week.
type WeeklyCount struct {
	Week  string `db:"week"` // strftime('%Y-W%W', ...)
	Count int    `db:"count"`
}

// MemoryAnalytics aggregates database-wide statistics for the analytics
// dashboard: overview counts, decay curve, access hotspots, and activity.
type MemoryAnalytics struct {
	TotalEntities     int
	TotalObservations int
	TotalRelations    int
	TotalSessions     int
	EmbeddingCoverage float64 // percent of valid observations with embeddings
	DBSizeBytes       int64
	DecayCurve        []AgeBucket
	TopAccessed       []AccessedObservation
	NeverAccessed     int
	FactTypeCounts    map[string]int
	SessionsPerWeek   []WeeklyCount // last 4 ISO weeks, oldest first
}

// GetMemoryAnalytics computes the aggregate statistics behind the Phase 6
// analytics dashboard (overview counts, decay curve, access hotspots,
// fact-type breakdown, and recent session activity).
func (s *Store) GetMemoryAnalytics(topN int) (*MemoryAnalytics, error) {
	a := &MemoryAnalytics{}

	if err := s.analyticsOverview(a); err != nil {
		return nil, err
	}

	decayCurve, err := s.decayCurve()
	if err != nil {
		return nil, err
	}
	a.DecayCurve = decayCurve

	topAccessed, err := s.topAccessed(topN)
	if err != nil {
		return nil, err
	}
	a.TopAccessed = topAccessed

	neverAccessed, err := s.neverAccessedCount()
	if err != nil {
		return nil, err
	}
	a.NeverAccessed = neverAccessed

	factTypeCounts, err := s.factTypeCounts()
	if err != nil {
		return nil, err
	}
	a.FactTypeCounts = factTypeCounts

	sessionsPerWeek, err := s.sessionsPerWeek()
	if err != nil {
		return nil, err
	}
	a.SessionsPerWeek = sessionsPerWeek

	coverage, err := s.embeddingCoverage()
	if err != nil {
		return nil, err
	}
	a.EmbeddingCoverage = coverage

	a.DBSizeBytes = s.dbSizeBytes()

	return a, nil
}

// analyticsOverview fills the basic entity/observation/relation/session
// counts on a.
func (s *Store) analyticsOverview(a *MemoryAnalytics) error {
	if err := s.db.Get(&a.TotalEntities, `SELECT COUNT(*) FROM entities WHERE is_latest = 1`); err != nil {
		return err
	}
	if err := s.db.Get(&a.TotalObservations, `
		SELECT COUNT(*) FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.valid_until IS NULL
	`); err != nil {
		return err
	}
	if err := s.db.Get(&a.TotalRelations, `SELECT COUNT(*) FROM relations`); err != nil {
		return err
	}
	if err := s.db.Get(&a.TotalSessions, `
		SELECT COUNT(*) FROM entities WHERE entity_type = 'session' AND is_latest = 1
	`); err != nil {
		return err
	}
	return nil
}

// decayCurve buckets valid observations on latest entities by age,
// filling zero-count buckets so the CLI can render all 4 ranges.
func (s *Store) decayCurve() ([]AgeBucket, error) {
	var rows []AgeBucket
	err := s.db.Select(&rows, `
		SELECT CASE
				WHEN age < 7 THEN '0-7d'
				WHEN age < 30 THEN '7-30d'
				WHEN age < 90 THEN '30-90d'
				ELSE '90d+'
			END AS label, COUNT(*) AS count, AVG(importance) AS avg_importance
		FROM (
			SELECT julianday('now') - julianday(o.created_at) AS age,
			       COALESCE(o.importance, 1.0) AS importance
			FROM observations o
			JOIN entities e ON e.id = o.entity_id
			WHERE e.is_latest = 1 AND o.valid_until IS NULL
		)
		GROUP BY label ORDER BY label
	`)
	if err != nil {
		return nil, err
	}

	byLabel := make(map[string]AgeBucket, len(rows))
	for _, r := range rows {
		byLabel[r.Label] = r
	}

	labels := []string{"0-7d", "7-30d", "30-90d", "90d+"}
	buckets := make([]AgeBucket, len(labels))
	for i, label := range labels {
		if r, ok := byLabel[label]; ok {
			buckets[i] = r
		} else {
			buckets[i] = AgeBucket{Label: label}
		}
	}
	return buckets, nil
}

// topAccessed returns the topN most-accessed valid observations on latest
// entities, ordered by access_count descending.
func (s *Store) topAccessed(topN int) ([]AccessedObservation, error) {
	var rows []AccessedObservation
	err := s.db.Select(&rows, `
		SELECT e.name AS entity_name, o.content, o.access_count
		FROM observations o JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.valid_until IS NULL AND o.access_count > 0
		ORDER BY o.access_count DESC LIMIT ?
	`, topN)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// neverAccessedCount counts valid observations on latest entities that
// have never been accessed (access_count = 0).
func (s *Store) neverAccessedCount() (int, error) {
	var count int
	err := s.db.Get(&count, `
		SELECT COUNT(*) FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.valid_until IS NULL AND o.access_count = 0
	`)
	return count, err
}

// factTypeCounts returns the count of valid observations on latest
// entities, grouped by fact_type (defaulting to 'dynamic' when unset).
func (s *Store) factTypeCounts() (map[string]int, error) {
	type row struct {
		FactType string `db:"fact_type"`
		Count    int    `db:"count"`
	}
	var rows []row
	err := s.db.Select(&rows, `
		SELECT COALESCE(o.fact_type, 'dynamic') AS fact_type, COUNT(*) AS count
		FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.valid_until IS NULL
		GROUP BY 1
	`)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]int, len(rows))
	for _, r := range rows {
		counts[r.FactType] = r.Count
	}
	return counts, nil
}

// sessionsPerWeek returns session-entity creation counts for the last 4
// ISO weeks, oldest first.
func (s *Store) sessionsPerWeek() ([]WeeklyCount, error) {
	var rows []WeeklyCount
	err := s.db.Select(&rows, `
		SELECT strftime('%Y-W%W', created_at) AS week, COUNT(*) AS count
		FROM entities
		WHERE entity_type = 'session' AND is_latest = 1
		AND created_at >= datetime('now', '-28 days')
		GROUP BY week ORDER BY week
	`)
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// embeddingCoverage returns the percentage of valid observations on latest
// entities that have an embedding.
func (s *Store) embeddingCoverage() (float64, error) {
	var total, embedded int
	if err := s.db.Get(&total, `
		SELECT COUNT(*) FROM observations o
		JOIN entities e ON e.id = o.entity_id
		WHERE e.is_latest = 1 AND o.valid_until IS NULL
	`); err != nil {
		return 0, err
	}
	if total == 0 {
		return 0, nil
	}
	if err := s.db.Get(&embedded, `
		SELECT COUNT(*) FROM observations o
		JOIN entities e ON e.id = o.entity_id
		JOIN observation_embeddings oe ON oe.observation_id = o.id
		WHERE e.is_latest = 1 AND o.valid_until IS NULL
	`); err != nil {
		return 0, err
	}
	return float64(embedded) / float64(total) * 100, nil
}

// dbSizeBytes returns the size of the database file on disk, or 0 if it
// cannot be stat'd (e.g. in-memory database).
func (s *Store) dbSizeBytes() int64 {
	info, err := os.Stat(s.path)
	if err != nil {
		return 0
	}
	return info.Size()
}
