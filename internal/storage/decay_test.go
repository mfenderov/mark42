package storage_test

import (
	"testing"
	"time"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestDecayConfig_Defaults(t *testing.T) {
	cfg := storage.DefaultDecayConfig()

	if cfg.ArchiveAfterDays <= 0 {
		t.Error("ArchiveAfterDays should be positive")
	}
	if cfg.ForgetAfterDays <= 0 {
		t.Error("ForgetAfterDays should be positive")
	}
	if cfg.MinImportanceToKeep < 0 || cfg.MinImportanceToKeep > 1 {
		t.Error("MinImportanceToKeep should be between 0 and 1")
	}
}

func TestStore_ApplySoftDecay(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity with observation
	store.CreateEntity("TDD", "pattern", []string{"Test-Driven Development"})
	store.SetObservationImportance("TDD", "Test-Driven Development", 0.5)

	// Apply soft decay
	affected, err := store.ApplySoftDecay(0.3)
	if err != nil {
		t.Fatalf("ApplySoftDecay failed: %v", err)
	}

	// Should have affected something (or not, depending on time)
	// This is a smoke test - the exact behavior depends on timestamps
	_ = affected
}

func TestStore_ForgetExpiredMemories(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity with observation
	store.CreateEntity("Expiring", "test", []string{"Will expire"})

	// Set forget_after to past using direct SQL to ensure it works
	_, err := store.DB().Exec(`
		UPDATE observations
		SET forget_after = datetime('now', '-1 hour')
		WHERE content = 'Will expire'
	`)
	if err != nil {
		t.Fatalf("Failed to set forget_after: %v", err)
	}

	// Forget expired
	deleted, err := store.ForgetExpiredMemories()
	if err != nil {
		t.Fatalf("ForgetExpiredMemories failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify observation is gone
	entity, err := store.GetEntity("Expiring")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}

	if len(entity.Observations) != 0 {
		t.Errorf("expected 0 observations after forget, got %d", len(entity.Observations))
	}
}

func TestStore_GetDecayStats(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create some test data
	store.CreateEntity("High", "test", []string{"High importance"})
	store.CreateEntity("Low", "test", []string{"Low importance"})
	store.SetObservationImportance("High", "High importance", 0.9)
	store.SetObservationImportance("Low", "Low importance", 0.1)

	stats, err := store.GetDecayStats()
	if err != nil {
		t.Fatalf("GetDecayStats failed: %v", err)
	}

	if stats.TotalObservations != 2 {
		t.Errorf("expected 2 total observations, got %d", stats.TotalObservations)
	}

	if stats.LowImportance != 1 {
		t.Errorf("expected 1 low importance observation, got %d", stats.LowImportance)
	}
}

func TestStore_ArchiveOldMemories(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Create entity with low importance observation
	store.CreateEntity("Old", "test", []string{"Old low importance memory"})
	store.SetObservationImportance("Old", "Old low importance memory", 0.05)

	// Manually set last_accessed to very old date
	_, err := store.DB().Exec(`
		UPDATE observations SET last_accessed = datetime('now', '-120 days')
		WHERE content = 'Old low importance memory'
	`)
	if err != nil {
		t.Fatalf("Failed to set old timestamp: %v", err)
	}

	cfg := storage.DefaultDecayConfig()
	cfg.ArchiveAfterDays = 90
	cfg.MinImportanceToKeep = 0.1

	archived, err := store.ArchiveOldMemories(cfg)
	if err != nil {
		t.Fatalf("ArchiveOldMemories failed: %v", err)
	}

	if archived != 1 {
		t.Errorf("expected 1 archived, got %d", archived)
	}

	// Check archive count
	archiveCount, _ := store.GetArchiveCount()
	if archiveCount != 1 {
		t.Errorf("expected 1 in archive, got %d", archiveCount)
	}
}

func TestStore_ForgetOldArchivedMemories(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	// Insert directly into archive with old date
	_, err := store.DB().Exec(`
		INSERT INTO archived_observations (original_entity_id, entity_name, content, archived_at)
		VALUES (1, 'OldArchive', 'Very old archived memory', datetime('now', '-200 days'))
	`)
	if err != nil {
		t.Fatalf("Failed to insert archive: %v", err)
	}

	// Forget old archived (180 days)
	deleted, err := store.ForgetOldArchivedMemories(180)
	if err != nil {
		t.Fatalf("ForgetOldArchivedMemories failed: %v", err)
	}

	if deleted != 1 {
		t.Errorf("expected 1 deleted from archive, got %d", deleted)
	}
}

func TestStore_SetForgetAfter(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	store.CreateEntity("Temporary", "test", []string{"Will be forgotten"})

	futureTime := time.Now().Add(24 * time.Hour)
	err := store.SetForgetAfter("Temporary", futureTime)
	if err != nil {
		t.Fatalf("SetForgetAfter failed: %v", err)
	}

	// Forget expired should not affect it yet
	deleted, err := store.ForgetExpiredMemories()
	if err != nil {
		t.Fatalf("ForgetExpiredMemories failed: %v", err)
	}

	if deleted != 0 {
		t.Errorf("expected 0 deleted (not expired yet), got %d", deleted)
	}
}
