package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
