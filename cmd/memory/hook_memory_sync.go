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

	if len(lines) == 0 || lines[0] != "---" {
		return nil, fmt.Errorf("no frontmatter found in %s", filepath.Base(path))
	}

	mem := &ccMemory{FileName: filepath.Base(path)}

	closingIdx := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			closingIdx = i
			break
		}
		parts := strings.SplitN(lines[i], ":", 2)
		if len(parts) != 2 {
			continue
		}
		value := strings.TrimSpace(parts[1])
		switch strings.TrimSpace(parts[0]) {
		case "name":
			mem.Name = value
		case "description":
			mem.Description = value
		case "type":
			mem.Type = value
		}
	}

	if closingIdx == -1 {
		return nil, fmt.Errorf("no closing frontmatter delimiter in %s", filepath.Base(path))
	}

	bodyLines := lines[closingIdx+1:]
	mem.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))

	return mem, nil
}
