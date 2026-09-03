package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mfenderov/mark42/internal/storage"
)

// PostToolUse runs the PostToolUse hook: track file modifications + session events.
func PostToolUse(projectDir string, input HookInput, opts ...Option) {
	cfg := loadPluginConfig(projectDir)

	hookCfg := &Config{}
	for _, o := range opts {
		o(hookCfg)
	}

	command, isGitCommit := bashCommand(input)
	if cfg.TriggerMode == "gitmode" && !isGitCommit {
		return
	}

	trackable := filterTrackable(filesFromInput(input, command, isGitCommit, projectDir), projectDir)

	m42 := mark42Dir(projectDir)
	_ = os.MkdirAll(m42, 0o755)

	recordSessionEvent(projectDir, input, command, trackable, hookCfg)
	updateDirtyFiles(m42, trackable)

	// CRITICAL: zero stdout output
}

// bashCommand extracts the trimmed command and whether it is a git commit.
func bashCommand(input HookInput) (command string, isGitCommit bool) {
	if input.ToolName != "Bash" {
		return "", false
	}
	cmd, ok := input.ToolInput["command"].(string)
	if !ok {
		return "", false
	}
	command = strings.TrimSpace(cmd)
	return command, strings.Contains(command, "git commit")
}

// filesFromInput determines which files a tool invocation touched.
func filesFromInput(input HookInput, command string, isGitCommit bool, projectDir string) []string {
	switch {
	case isGitCommit:
		// Git commit file extraction requires subprocess — skip for now,
		// just track the event
		return nil
	case input.ToolName == "Bash":
		return extractFilesFromBash(command, projectDir)
	default:
		return filePathArg(input)
	}
}

func filePathArg(input HookInput) []string {
	if fp, ok := input.ToolInput["file_path"].(string); ok && fp != "" {
		return []string{fp}
	}
	return nil
}

func filterTrackable(files []string, projectDir string) []string {
	var trackable []string
	for _, f := range files {
		if shouldTrack(f, projectDir) {
			trackable = append(trackable, f)
		}
	}
	return trackable
}

// recordSessionEvent writes the session event to SQLite when a current session exists.
func recordSessionEvent(projectDir string, input HookInput, command string, trackable []string, hookCfg *Config) {
	sessionName := readCurrentSession(projectDir)
	if sessionName == "" {
		return
	}

	se := storage.SessionEvent{
		ToolName:  input.ToolName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	switch {
	case (input.ToolName == "Edit" || input.ToolName == "Write") && len(trackable) > 0:
		se.FilePath = trackable[0]
	case input.ToolName == "Bash" && command != "":
		se.Command = truncateCommand(command, 200)
	}

	s, owned := getOrUseStore(hookCfg)
	if s == nil {
		return
	}
	if owned {
		defer s.Close()
	}
	_ = s.CaptureSessionEvent(sessionName, se)
}

// truncateCommand cuts a command to n bytes (plain cut, no ellipsis).
func truncateCommand(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// updateDirtyFiles merges newly modified files into the dirty-files list.
func updateDirtyFiles(m42 string, trackable []string) {
	if len(trackable) == 0 {
		return
	}

	dirtyPath := filepath.Join(m42, "dirty-files")
	existing := make(map[string]string)
	for _, line := range readLines(dirtyPath) {
		path := line
		if before, _, found := strings.Cut(line, " ["); found {
			path = before
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

func loadPluginConfig(projectDir string) PluginConfig {
	data, err := os.ReadFile(filepath.Join(mark42Dir(projectDir), "config.json"))
	if err != nil {
		return PluginConfig{TriggerMode: "default"}
	}
	var cfg PluginConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return PluginConfig{TriggerMode: "default"}
	}
	if cfg.TriggerMode == "" {
		cfg.TriggerMode = "default"
	}
	return cfg
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
	if command == "" || isReadOnlyCommand(command) {
		return nil
	}

	tokens := shellTokenize(command)
	if len(tokens) == 0 {
		return nil
	}

	var files []string
	switch cmd := tokens[0]; {
	case cmd == "rm":
		files = fileArgs(tokens[1:])
	case cmd == "git" && len(tokens) > 1 && tokens[1] == "rm":
		files = fileArgs(tokens[2:])
	case cmd == "mv" && len(tokens) >= 3:
		files = firstFileArg(tokens[1:])
	case cmd == "git" && len(tokens) > 2 && tokens[1] == "mv":
		files = firstFileArg(tokens[2:])
	case cmd == "unlink" && len(tokens) > 1 && !isShellSyntax(tokens[1]):
		files = tokens[1:2]
	}

	return resolvePaths(files, projectDir)
}

var readOnlyPrefixes = []string{
	"ls", "cat", "echo", "grep", "find", "head", "tail", "less", "more",
	"cd", "pwd", "which", "whereis", "type", "file", "stat", "wc",
	"git status", "git log", "git diff", "git show", "git branch",
	"git fetch", "git pull", "git push", "git clone", "git checkout",
	"git stash", "git remote", "git tag", "git rev-parse",
	"npm ", "yarn ", "pnpm ", "node ", "python", "pip ", "uv ",
	"cargo ", "go ", "make", "cmake", "docker ", "kubectl ",
	"curl ", "wget ", "ssh ", "scp ", "rsync ",
}

func isReadOnlyCommand(command string) bool {
	for _, prefix := range readOnlyPrefixes {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}

// fileArgs returns non-flag tokens up to the first shell operator or redirect.
func fileArgs(tokens []string) []string {
	var files []string
	for _, tok := range tokens {
		if isShellSyntax(tok) {
			break
		}
		if !strings.HasPrefix(tok, "-") {
			files = append(files, tok)
		}
	}
	return files
}

// firstFileArg returns the first non-flag token, stopping at shell syntax.
func firstFileArg(tokens []string) []string {
	if args := fileArgs(tokens); len(args) > 0 {
		return args[:1]
	}
	return nil
}

func resolvePaths(files []string, projectDir string) []string {
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
