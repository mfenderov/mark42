package claude

import (
	"encoding/json"
	"path/filepath"
	"strconv"
)

// PreCompact runs the PreCompact hook: report tracked files.
func PreCompact(projectDir string, opts ...Option) {
	cfg := &Config{}
	for _, o := range opts {
		o(cfg)
	}

	if projectDir == "" {
		return
	}

	projectName := filepath.Base(projectDir)
	files := readLines(filepath.Join(mark42Dir(projectDir), "dirty-files"))

	output := map[string]any{
		"systemMessage": "mark42: pre-compact — " + strconv.Itoa(len(files)) + " files tracked for " + projectName,
	}

	data, _ := json.Marshal(output)
	printHook(cfg, string(data))
}
