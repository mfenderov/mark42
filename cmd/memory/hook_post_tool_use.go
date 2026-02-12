package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type hookInput struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

type pluginConfig struct {
	TriggerMode string `json:"triggerMode"`
}

var hookPostToolUseCmd = &cobra.Command{
	Use:   "post-tool-use",
	Short: "PostToolUse hook: track file modifications",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		var input hookInput
		if err := readStdinJSON(&input); err != nil {
			return nil
		}

		runPostToolUseHook(projectDir, input)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookPostToolUseCmd)
}

func loadPluginConfig(projectDir string) pluginConfig {
	data, err := os.ReadFile(filepath.Join(mark42Dir(projectDir), "config.json"))
	if err != nil {
		return pluginConfig{TriggerMode: "default"}
	}
	var cfg pluginConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return pluginConfig{TriggerMode: "default"}
	}
	if cfg.TriggerMode == "" {
		cfg.TriggerMode = "default"
	}
	return cfg
}

func runPostToolUseHook(projectDir string, input hookInput) {
	cfg := loadPluginConfig(projectDir)

	command := ""
	isGitCommit := false
	if input.ToolName == "Bash" {
		if cmd, ok := input.ToolInput["command"].(string); ok {
			command = strings.TrimSpace(cmd)
			isGitCommit = strings.Contains(command, "git commit")
		}
	}

	if cfg.TriggerMode == "gitmode" && !isGitCommit {
		return
	}

	var filesToTrack []string

	switch {
	case isGitCommit:
		// Git commit file extraction requires subprocess — skip for now,
		// just track the event
	case input.ToolName == "Edit" || input.ToolName == "Write":
		if fp, ok := input.ToolInput["file_path"].(string); ok && fp != "" {
			filesToTrack = append(filesToTrack, fp)
		}
	case input.ToolName == "Bash":
		filesToTrack = extractFilesFromBash(command, projectDir)
	default:
		if fp, ok := input.ToolInput["file_path"].(string); ok && fp != "" {
			filesToTrack = append(filesToTrack, fp)
		}
	}

	if len(filesToTrack) == 0 && !isGitCommit {
		return
	}

	var trackable []string
	for _, f := range filesToTrack {
		if shouldTrack(f, projectDir) {
			trackable = append(trackable, f)
		}
	}

	if len(trackable) == 0 && !isGitCommit {
		return
	}

	m42 := mark42Dir(projectDir)
	_ = os.MkdirAll(m42, 0o755)

	// Update dirty-files (deduplicated)
	if len(trackable) > 0 {
		dirtyPath := filepath.Join(m42, "dirty-files")
		existing := make(map[string]string)
		for _, line := range readLines(dirtyPath) {
			path := line
			if idx := strings.Index(line, " ["); idx != -1 {
				path = line[:idx]
			}
			existing[path] = line
		}

		for _, fp := range trackable {
			if _, ok := existing[fp]; !ok {
				existing[fp] = fp
			}
		}

		var sb strings.Builder
		for _, line := range existing {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
		_ = os.WriteFile(dirtyPath, []byte(sb.String()), 0o644)
	}

	// Write session event (JSONL)
	event := map[string]string{
		"toolName":  input.ToolName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	if (input.ToolName == "Edit" || input.ToolName == "Write") && len(trackable) > 0 {
		event["filePath"] = trackable[0]
	} else if input.ToolName == "Bash" && command != "" {
		cmd := command
		if len(cmd) > 200 {
			cmd = cmd[:200]
		}
		event["command"] = cmd
	}

	eventJSON, _ := json.Marshal(event)
	eventsPath := filepath.Join(m42, "session-events")
	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err == nil {
		f.Write(eventJSON)
		f.Write([]byte("\n"))
		f.Close()
	}

	// CRITICAL: zero stdout output
}

func shouldTrack(filePath, projectDir string) bool {
	if !strings.HasPrefix(filePath, projectDir) {
		return false
	}

	rel, err := filepath.Rel(projectDir, filePath)
	if err != nil {
		return false
	}

	parts := strings.SplitN(rel, string(filepath.Separator), 2)
	if len(parts) > 0 && parts[0] == ".claude" {
		return false
	}

	if filepath.Base(filePath) == "CLAUDE.md" {
		return false
	}

	return true
}

func extractFilesFromBash(command, projectDir string) []string {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil
	}

	skipPrefixes := []string{
		"ls", "cat", "echo", "grep", "find", "head", "tail", "less", "more",
		"cd", "pwd", "which", "whereis", "type", "file", "stat", "wc",
		"git status", "git log", "git diff", "git show", "git branch",
		"git fetch", "git pull", "git push", "git clone", "git checkout",
		"git stash", "git remote", "git tag", "git rev-parse",
		"npm ", "yarn ", "pnpm ", "node ", "python", "pip ", "uv ",
		"cargo ", "go ", "make", "cmake", "docker ", "kubectl ",
		"curl ", "wget ", "ssh ", "scp ", "rsync ",
	}
	for _, prefix := range skipPrefixes {
		if strings.HasPrefix(command, prefix) {
			return nil
		}
	}

	tokens := shellTokenize(command)
	if len(tokens) == 0 {
		return nil
	}

	var files []string
	cmd := tokens[0]

	switch {
	case cmd == "rm":
		for _, tok := range tokens[1:] {
			if isShellSyntax(tok) {
				break
			}
			if !strings.HasPrefix(tok, "-") {
				files = append(files, tok)
			}
		}

	case cmd == "git" && len(tokens) > 1 && tokens[1] == "rm":
		for _, tok := range tokens[2:] {
			if isShellSyntax(tok) {
				break
			}
			if !strings.HasPrefix(tok, "-") {
				files = append(files, tok)
			}
		}

	case cmd == "mv" && len(tokens) >= 3:
		for _, tok := range tokens[1:] {
			if isShellSyntax(tok) {
				break
			}
			if !strings.HasPrefix(tok, "-") {
				files = append(files, tok)
				break
			}
		}

	case cmd == "git" && len(tokens) > 2 && tokens[1] == "mv":
		for _, tok := range tokens[2:] {
			if isShellSyntax(tok) {
				break
			}
			if !strings.HasPrefix(tok, "-") {
				files = append(files, tok)
				break
			}
		}

	case cmd == "unlink" && len(tokens) > 1:
		if !isShellSyntax(tokens[1]) {
			files = append(files, tokens[1])
		}
	}

	var resolved []string
	for _, f := range files {
		if !filepath.IsAbs(f) {
			f = filepath.Join(projectDir, f)
		}
		resolved = append(resolved, filepath.Clean(f))
	}
	return resolved
}

func shellTokenize(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	var tokens []string
	var current strings.Builder
	inSingle := false
	inDouble := false

	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '\'' && !inDouble:
			inSingle = !inSingle
		case ch == '"' && !inSingle:
			inDouble = !inDouble
		case ch == ' ' && !inSingle && !inDouble:
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

var shellOperators = map[string]bool{
	"&&": true, "||": true, ";": true, "|": true,
}

var redirectPrefixes = []string{">", ">>", "<", "2>", "2>>", "1>", "1>>", "2>&1"}

func isShellSyntax(token string) bool {
	if shellOperators[token] {
		return true
	}
	for _, prefix := range redirectPrefixes {
		if strings.HasPrefix(token, prefix) {
			return true
		}
	}
	return false
}
