package claude

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mfenderov/mark42/internal/state"
	"github.com/mfenderov/mark42/internal/storage"
)

type ccMemory struct {
	Name        string
	Description string
	Type        string
	Body        string
	FileName    string
}

func parseCCMemoryFile(path string) (*ccMemory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read cc memory file: %w", err)
	}
	return parseCCMemoryBytes(data, filepath.Base(path))
}

func parseCCMemoryBytes(data []byte, fileName string) (*ccMemory, error) {
	lines := strings.Split(string(data), "\n")

	if lines[0] != "---" {
		return nil, fmt.Errorf("no frontmatter found in %s", fileName)
	}

	mem := &ccMemory{FileName: fileName}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closingIdx = i
			break
		}
		key, value, ok := strings.Cut(lines[i], ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "name":
			mem.Name = strings.TrimSpace(value)
		case "description":
			mem.Description = strings.TrimSpace(value)
		case "type":
			mem.Type = strings.TrimSpace(value)
		}
	}

	if closingIdx == -1 {
		return nil, fmt.Errorf("no closing frontmatter delimiter in %s", fileName)
	}

	if strings.TrimSpace(mem.Name) == "" {
		return nil, fmt.Errorf("missing required 'name' frontmatter in %s", fileName)
	}
	if strings.TrimSpace(mem.Type) == "" {
		return nil, fmt.Errorf("missing required 'type' frontmatter in %s", fileName)
	}

	bodyLines := lines[closingIdx+1:]
	mem.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	return mem, nil
}

func loadChecksums(path string) map[string]string {
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	var checksums map[string]string
	if json.Unmarshal(data, &checksums) != nil {
		return map[string]string{}
	}
	return checksums
}

func saveChecksums(path string, checksums map[string]string) {
	data, err := json.Marshal(checksums)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		logger.Warn("failed to create checksum dir", "err", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		logger.Warn("failed to save checksums", "err", err)
	}
}

func checksumBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ccMemoryDir(projectDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", state.ProjectSlug(projectDir), "memory")
}

func syncCCMemory(projectName, memoryDir string, store *storage.Store, checksumPath string) {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warn("failed to read cc memory dir", "dir", memoryDir, "err", err)
		}
		return
	}

	checksums := loadChecksums(checksumPath)
	changed := false

	// Ensure project entity exists once before processing files
	projectEntityName := "project:" + projectName
	_, projErr := store.CreateEntity(projectEntityName, "project", nil)
	if projErr != nil && !errors.Is(projErr, storage.ErrEntityExists) {
		logger.Warn("failed to create project entity, aborting cc memory sync",
			"entity", projectEntityName, "err", projErr)
		return
	}

	for _, entry := range entries {
		if entry.IsDir() || !isMemoryFile(entry.Name()) {
			continue
		}
		if syncMemoryFile(store, projectName, projectEntityName, memoryDir, entry.Name(), checksums) {
			changed = true
		}
	}

	if changed {
		saveChecksums(checksumPath, checksums)
	}
}

// isMemoryFile reports whether name is a syncable memory file.
func isMemoryFile(name string) bool {
	return !strings.EqualFold(name, "MEMORY.md") && strings.HasSuffix(name, ".md")
}

// syncMemoryFile syncs one memory file; reports whether its checksum changed.
func syncMemoryFile(store *storage.Store, projectName, projectEntityName, memoryDir, name string, checksums map[string]string) bool {
	data, err := os.ReadFile(filepath.Join(memoryDir, name))
	if err != nil {
		logger.Warn("failed to read cc memory file", "file", name, "err", err)
		return false
	}

	sum := checksumBytes(data)
	if checksums[name] == sum {
		return false
	}

	mem, err := parseCCMemoryBytes(data, name)
	if err != nil {
		logger.Warn("failed to parse cc memory file", "file", name, "err", err)
		return false
	}

	entityName := "cc-memory/" + projectName + "/" + mem.Name
	if err := upsertMemoryEntity(store, entityName, projectEntityName, mem); err != nil {
		return false
	}

	checksums[name] = sum
	return true
}

// upsertMemoryEntity creates/updates the memory entity with its observations
// and project relation. Relation failures are logged but non-fatal.
func upsertMemoryEntity(store *storage.Store, entityName, projectEntityName string, mem *ccMemory) error {
	if _, err := store.CreateOrUpdateEntity(entityName, mem.Type, nil); err != nil {
		logger.Warn("failed to upsert entity", "entity", entityName, "err", err)
		return err
	}

	if mem.Description != "" {
		if err := store.AddObservationWithType(entityName, mem.Description, storage.FactTypeStatic); err != nil {
			logger.Warn("failed to add description observation", "entity", entityName, "err", err)
			return err
		}
	}

	if mem.Body != "" {
		if err := store.AddObservationWithType(entityName, mem.Body, storage.FactTypeDynamic); err != nil {
			logger.Warn("failed to add body observation", "entity", entityName, "err", err)
			return err
		}
	}

	if err := store.CreateRelation(entityName, projectEntityName, "belongs_to"); err != nil {
		logger.Warn("failed to create belongs_to relation", "from", entityName, "to", projectEntityName, "err", err)
	}
	return nil
}
