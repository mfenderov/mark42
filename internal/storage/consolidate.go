package storage

import (
	"fmt"
	"strings"
)

// ConsolidateObservations merges duplicate or near-duplicate observations for an entity.
// Uses string containment as a similarity heuristic: if one observation is a substring
// of another, the shorter one is removed (the longer one is more comprehensive).
// Returns a summary of what was consolidated.
func (s *Store) ConsolidateObservations(entityName string) (string, error) {
	entity, err := s.GetEntity(entityName)
	if err != nil {
		return "", fmt.Errorf("entity not found: %w", err)
	}

	if len(entity.Observations) <= 1 {
		return fmt.Sprintf("%s: nothing to consolidate (%d observations)", entityName, len(entity.Observations)), nil
	}

	// Find observations where one is a substring of another
	var toDelete []string
	observations := entity.Observations

	for i := 0; i < len(observations); i++ {
		for j := i + 1; j < len(observations); j++ {
			lowerI := strings.ToLower(observations[i])
			lowerJ := strings.ToLower(observations[j])

			if strings.Contains(lowerJ, lowerI) {
				// observations[i] is contained in observations[j] — remove the shorter one
				toDelete = append(toDelete, observations[i])
			} else if strings.Contains(lowerI, lowerJ) {
				// observations[j] is contained in observations[i] — remove the shorter one
				toDelete = append(toDelete, observations[j])
			}
		}
	}

	// Deduplicate toDelete list
	seen := make(map[string]bool)
	var uniqueDeletes []string
	for _, d := range toDelete {
		if !seen[d] {
			seen[d] = true
			uniqueDeletes = append(uniqueDeletes, d)
		}
	}

	// Delete the duplicates
	deleted := 0
	for _, obs := range uniqueDeletes {
		if err := s.DeleteObservation(entityName, obs); err == nil {
			deleted++
		}
	}

	return fmt.Sprintf("%s: consolidated %d redundant observations (kept %d)",
		entityName, deleted, len(observations)-deleted), nil
}
