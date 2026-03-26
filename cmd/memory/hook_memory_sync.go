package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mfenderov/mark42/internal/storage"
)

type ccMemory struct {
	Name        string
	Description string
	Type        string
	Body        string
	FileName    string
}

func projectSlug(projectDir string) string {
	return strings.ReplaceAll(projectDir, "/", "-")
}

func parseCCMemoryFile(path string) (*ccMemory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading cc memory file: %w", err)
	}

	lines := strings.Split(string(data), "\n")

	if lines[0] != "---" {
		return nil, fmt.Errorf("no frontmatter found in %s", filepath.Base(path))
	}

	mem := &ccMemory{FileName: filepath.Base(path)}

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
		return nil, fmt.Errorf("no closing frontmatter delimiter in %s", filepath.Base(path))
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
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, data, 0o644)
}

func fileChecksum(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ccMemoryDir(projectDir string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects", projectSlug(projectDir), "memory")
}

func syncCCMemory(projectSlugName, memoryDir string, store *storage.Store, checksumPath string) {
	entries, err := os.ReadDir(memoryDir)
	if err != nil {
		return
	}

	checksums := loadChecksums(checksumPath)
	changed := false

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		if strings.EqualFold(name, "MEMORY.md") {
			continue
		}

		if !strings.HasSuffix(name, ".md") {
			continue
		}

		path := filepath.Join(memoryDir, name)

		sum := fileChecksum(path)
		if sum == "" {
			continue
		}
		if checksums[name] == sum {
			continue
		}

		mem, err := parseCCMemoryFile(path)
		if err != nil {
			logger.Warn("failed to parse cc memory file", "file", name, "err", err)
			continue
		}

		entityName := "cc-memory/" + projectSlugName + "/" + mem.Name

		_, err = store.CreateOrUpdateEntity(entityName, mem.Type, nil)
		if err != nil {
			logger.Warn("failed to upsert entity", "entity", entityName, "err", err)
			continue
		}

		if mem.Description != "" {
			_ = store.AddObservationWithType(entityName, mem.Description, storage.FactTypeStatic)
		}

		if mem.Body != "" {
			_ = store.AddObservationWithType(entityName, mem.Body, storage.FactTypeDynamic)
		}

		checksums[name] = sum
		changed = true
	}

	if changed {
		saveChecksums(checksumPath, checksums)
	}
}
