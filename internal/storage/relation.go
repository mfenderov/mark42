package storage

import "time"

// Relation represents an edge between two entities.
type Relation struct {
	From      string
	To        string
	Type      string
	CreatedAt time.Time
}

// CreateRelation creates a relation between two entities.
func (s *Store) CreateRelation(fromName, toName, relationType string) error {
	// Get entity IDs
	var fromID, toID int64

	err := s.db.QueryRow("SELECT id FROM entities WHERE name = ?", fromName).Scan(&fromID)
	if err != nil {
		return ErrNotFound
	}

	err = s.db.QueryRow("SELECT id FROM entities WHERE name = ?", toName).Scan(&toID)
	if err != nil {
		return ErrNotFound
	}

	// Insert relation (ignore duplicate)
	_, err = s.db.Exec(
		"INSERT OR IGNORE INTO relations (from_entity_id, to_entity_id, relation_type) VALUES (?, ?, ?)",
		fromID, toID, relationType,
	)
	return err
}

// ListRelations returns all relations involving an entity (both directions).
func (s *Store) ListRelations(entityName string) ([]*Relation, error) {
	var entityID int64
	err := s.db.QueryRow("SELECT id FROM entities WHERE name = ?", entityName).Scan(&entityID)
	if err != nil {
		return nil, ErrNotFound
	}

	// Query both outgoing and incoming relations
	rows, err := s.db.Query(`
		SELECT e_from.name, e_to.name, r.relation_type, r.created_at
		FROM relations r
		JOIN entities e_from ON r.from_entity_id = e_from.id
		JOIN entities e_to ON r.to_entity_id = e_to.id
		WHERE r.from_entity_id = ? OR r.to_entity_id = ?
		ORDER BY r.created_at
	`, entityID, entityID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var relations []*Relation
	for rows.Next() {
		var r Relation
		if err := rows.Scan(&r.From, &r.To, &r.Type, &r.CreatedAt); err != nil {
			return nil, err
		}
		relations = append(relations, &r)
	}

	return relations, nil
}

// DeleteRelation removes a specific relation.
func (s *Store) DeleteRelation(fromName, toName, relationType string) error {
	var fromID, toID int64

	err := s.db.QueryRow("SELECT id FROM entities WHERE name = ?", fromName).Scan(&fromID)
	if err != nil {
		return ErrNotFound
	}

	err = s.db.QueryRow("SELECT id FROM entities WHERE name = ?", toName).Scan(&toID)
	if err != nil {
		return ErrNotFound
	}

	result, err := s.db.Exec(
		"DELETE FROM relations WHERE from_entity_id = ? AND to_entity_id = ? AND relation_type = ?",
		fromID, toID, relationType,
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
