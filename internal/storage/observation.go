package storage

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
