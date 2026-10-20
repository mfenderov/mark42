# Async Hooks Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make PostToolUse and Stop hooks async, eliminate the session-events flat file by writing events directly to SQLite in real time, and expand PostToolUse coverage to all tools.

**Architecture:** SessionStart creates a pending session in SQLite and writes its name to `.claude/mark42/current-session`. PostToolUse reads that file and appends events to SQLite directly. Stop reads the file, completes the session with a summary, then deletes the file.

**Tech Stack:** Go, SQLite (via sqlx), cobra — no new dependencies.

---

## File Map

| File | Change |
|------|--------|
| `hooks/hooks.json` | Add `"async": true` to PostToolUse and Stop; change PostToolUse matcher to `".*"` |
| `cmd/memory/hooks.go` | Add `readCurrentSession` helper |
| `cmd/memory/hook_session_start.go` | Add `store *storage.Store` to `hookConfig`; add `withStore` hookOption; after context injection, call `store.CreateSession` and write `current-session` file |
| `cmd/memory/hook_session_start_test.go` | Add two new tests for current-session file creation |
| `cmd/memory/hook_post_tool_use.go` | Add `opts ...hookOption` param; replace session-events flat file append with `store.CaptureSessionEvent`; add `storage` import |
| `cmd/memory/hook_post_tool_use_test.go` | Update three tests that check session-events flat file; add two SQLite event tests |
| `cmd/memory/hook_stop.go` | Read `current-session` file; replace `captureSessionDirectly` with `completeSession`; simplify `buildAutoSummary` to remove events param; remove session-events reads/clears; add `os.Remove(currentSessionFile)` |
| `cmd/memory/hook_stop_test.go` | Remove session-events setup/assertions from three tests; add two new session-completion tests; update `buildAutoSummary` test calls |

---

### Task 1: Worktree + Config + Helpers

**Files:**
- Modify: `hooks/hooks.json`
- Modify: `cmd/memory/hooks.go`

- [ ] **Step 1: Create worktree**

```bash
grep -q '\.worktrees' .gitignore 2>/dev/null || echo '.worktrees/' >> .gitignore
git worktree add .worktrees/feat/async-hooks -b feat/async-hooks
cd .worktrees/feat/async-hooks
```

- [ ] **Step 2: Update hooks.json**

Replace the contents of `hooks/hooks.json` with:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup|resume",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PLUGIN_ROOT}/bin/hook.sh\" hook session-start",
            "timeout": 120
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": ".*",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PLUGIN_ROOT}/bin/hook.sh\" hook post-tool-use",
            "timeout": 15,
            "async": true
          }
        ]
      }
    ],
    "Stop": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PLUGIN_ROOT}/bin/hook.sh\" hook stop",
            "timeout": 120,
            "async": true
          }
        ]
      }
    ],
    "PreCompact": [
      {
        "matcher": "manual|auto",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"${CLAUDE_PLUGIN_ROOT}/bin/hook.sh\" hook pre-compact",
            "timeout": 120
          }
        ]
      }
    ]
  }
}
```

- [ ] **Step 3: Add `readCurrentSession` to `cmd/memory/hooks.go`**

Add after the `clearFlag` function (line 77):

```go
func readCurrentSession(projectDir string) string {
	data, err := os.ReadFile(filepath.Join(mark42Dir(projectDir), "current-session"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
```

- [ ] **Step 4: Commit**

```bash
git add hooks/hooks.json cmd/memory/hooks.go
git commit -m "feat: async hooks config + readCurrentSession helper"
```

---

### Task 2: SessionStart Creates Pending Session

**Files:**
- Modify: `cmd/memory/hook_session_start.go`
- Modify: `cmd/memory/hook_session_start_test.go`

- [ ] **Step 1: Write failing tests**

Add to the bottom of `TestHookSessionStart` in `cmd/memory/hook_session_start_test.go` (before the closing `}`):

```go
	t.Run("creates current-session file when store is not nil", func(t *testing.T) {
		dir := t.TempDir()
		projectDir := filepath.Join(dir, "testproject")
		os.MkdirAll(mark42Dir(projectDir), 0o755)
		dbPath := filepath.Join(dir, "test.db")
		store, err := storage.NewStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		store.Migrate()

		var buf captureBuffer
		runSessionStartHook(projectDir, store, withOutput(&buf))

		data, err := os.ReadFile(filepath.Join(mark42Dir(projectDir), "current-session"))
		if err != nil {
			t.Fatalf("current-session file not created: %v", err)
		}
		if !strings.HasPrefix(strings.TrimSpace(string(data)), "session-testproject-") {
			t.Errorf("unexpected session name: %q", strings.TrimSpace(string(data)))
		}
	})

	t.Run("no current-session file when store is nil", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(mark42Dir(dir), 0o755)

		var buf captureBuffer
		runSessionStartHook(dir, nil, withOutput(&buf))

		if _, err := os.Stat(filepath.Join(mark42Dir(dir), "current-session")); !os.IsNotExist(err) {
			t.Error("current-session should not be created when store is nil")
		}
	})
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./cmd/memory/... -run TestHookSessionStart -v 2>&1 | tail -20
```

Expected: `FAIL — current-session file not created`

- [ ] **Step 3: Add `store` field + `withStore` hookOption to `hookConfig`**

In `cmd/memory/hook_session_start.go`, update `hookConfig` and add `withStore`:

```go
type hookConfig struct {
	writer    *captureBuffer
	stopInput *stopInput
	store     *storage.Store
}

func withStore(s *storage.Store) hookOption {
	return func(cfg *hookConfig) {
		cfg.store = s
	}
}
```

- [ ] **Step 4: Write current-session file in `runSessionStartHook`**

In `cmd/memory/hook_session_start.go`, add at the end of `runSessionStartHook`, after the `hookPrint(cfg, combined)` call:

```go
	// Create pending session and record its name for PostToolUse and Stop hooks
	session, err := store.CreateSession(projectName)
	if err == nil {
		_ = os.WriteFile(
			filepath.Join(mark42Dir(projectDir), "current-session"),
			[]byte(session.Name),
			0o644,
		)
	}
```

Also add `"os"` to the imports in `hook_session_start.go` (it's not currently imported there):

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)
```

- [ ] **Step 5: Run to verify pass**

```bash
go test ./cmd/memory/... -run TestHookSessionStart -v 2>&1 | tail -20
```

Expected: all `TestHookSessionStart` subtests PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/memory/hook_session_start.go cmd/memory/hook_session_start_test.go
git commit -m "feat: SessionStart creates pending session and writes current-session file"
```

---

### Task 3: PostToolUse Writes Events to SQLite

**Files:**
- Modify: `cmd/memory/hook_post_tool_use.go`
- Modify: `cmd/memory/hook_post_tool_use_test.go`

- [ ] **Step 1: Write failing tests**

In `cmd/memory/hook_post_tool_use_test.go`:

**a) Add two new tests at the bottom of `TestPostToolUseHook` (before closing `}`):**

```go
	t.Run("writes event to SQLite when current-session present", func(t *testing.T) {
		dir := setupProjectDir(t)
		dbPath := filepath.Join(t.TempDir(), "test.db")
		store, err := storage.NewStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		store.Migrate()

		session, _ := store.CreateSession(filepath.Base(dir))
		os.WriteFile(filepath.Join(mark42Dir(dir), "current-session"), []byte(session.Name), 0o644)

		input := hookInput{
			ToolName:  "Read",
			ToolInput: map[string]any{},
		}
		runPostToolUseHook(dir, input, withStore(store))

		got, err := store.GetSession(session.Name)
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if got.EventCount != 1 {
			t.Errorf("expected 1 event in SQLite, got %d", got.EventCount)
		}
	})

	t.Run("skips SQLite when current-session absent", func(t *testing.T) {
		dir := setupProjectDir(t)
		dbPath := filepath.Join(t.TempDir(), "test.db")
		store, err := storage.NewStore(dbPath)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		store.Migrate()

		input := hookInput{
			ToolName:  "Read",
			ToolInput: map[string]any{},
		}
		runPostToolUseHook(dir, input, withStore(store))

		sessions, _ := store.ListSessions("", "", 10)
		if len(sessions) != 0 {
			t.Errorf("no sessions should exist when current-session absent, got %d", len(sessions))
		}
	})
```

**b) Add `storage` import to the test file:**

```go
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./cmd/memory/... -run TestPostToolUseHook/writes_event_to_SQLite -v 2>&1 | tail -10
```

Expected: compile error — `withStore` undefined in this package context / `runPostToolUseHook` wrong number of args

- [ ] **Step 3: Update `runPostToolUseHook` to accept opts and write to SQLite**

Replace `cmd/memory/hook_post_tool_use.go` entirely with:

```go
package main

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
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
	if err := unmarshalJSON(data, &cfg); err != nil {
		return pluginConfig{TriggerMode: "default"}
	}
	if cfg.TriggerMode == "" {
		cfg.TriggerMode = "default"
	}
	return cfg
}

func runPostToolUseHook(projectDir string, input hookInput, opts ...hookOption) {
	cfg := &hookConfig{}
	for _, o := range opts {
		o(cfg)
	}

	plugCfg := loadPluginConfig(projectDir)

	command := ""
	isGitCommit := false
	if input.ToolName == "Bash" {
		if cmd, ok := input.ToolInput["command"].(string); ok {
			command = strings.TrimSpace(cmd)
			isGitCommit = strings.Contains(command, "git commit")
		}
	}

	if plugCfg.TriggerMode == "gitmode" && !isGitCommit {
		return
	}

	var filesToTrack []string

	switch {
	case isGitCommit:
		// Git commit file extraction requires subprocess — skip for now
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

	var trackable []string
	for _, f := range filesToTrack {
		if shouldTrack(f, projectDir) {
			trackable = append(trackable, f)
		}
	}

	m42 := mark42Dir(projectDir)
	_ = os.MkdirAll(m42, 0o755)

	// Build session event
	se := storage.SessionEvent{
		ToolName:  input.ToolName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	if (input.ToolName == "Edit" || input.ToolName == "Write") && len(trackable) > 0 {
		se.FilePath = trackable[0]
	} else if input.ToolName == "Bash" && command != "" {
		cmd := command
		if len(cmd) > 200 {
			cmd = cmd[:200]
		}
		se.Command = cmd
	}

	// Write event to SQLite via current session
	if sessionName := readCurrentSession(projectDir); sessionName != "" {
		s := cfg.store
		if s == nil {
			s, _ = getStore()
			if s != nil {
				defer s.Close()
			}
		}
		if s != nil {
			_ = s.CaptureSessionEvent(sessionName, se)
		}
	}

	// Update dirty-files (only when files were modified)
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
```

Note: `unmarshalJSON` used in `loadPluginConfig` — replace with `json.Unmarshal` and add `"encoding/json"` back to imports:

```go
import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)
```

And in `loadPluginConfig`, use `json.Unmarshal(data, &cfg)` (not `unmarshalJSON`).

- [ ] **Step 4: Update tests that referenced session-events flat file**

In `cmd/memory/hook_post_tool_use_test.go`, update three tests:

**"tracks Edit file"** — remove the events check (lines after `dirty` assertion):
```go
	t.Run("tracks Edit file", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, "src", "main.go")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 1 {
			t.Fatalf("got %d dirty files, want 1", len(dirty))
		}
		if !strings.Contains(dirty[0], "main.go") {
			t.Errorf("dirty file should contain main.go, got %q", dirty[0])
		}
	})
```

**"writes session events as JSONL"** — delete this test entirely (covered by new SQLite test above).

**"read-only Bash writes event but no dirty files"** — remove session-events assertions:
```go
	t.Run("read-only Bash writes event but no dirty files", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Bash",
			ToolInput: map[string]any{"command": "go test ./..."},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 0 {
			t.Errorf("read-only Bash should not create dirty files, got %d", len(dirty))
		}
	})
```

**"excluded Edit writes event but no dirty files"** — remove session-events assertions:
```go
	t.Run("excluded Edit writes event but no dirty files", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := hookInput{
			ToolName:  "Edit",
			ToolInput: map[string]any{"file_path": filepath.Join(dir, ".claude", "config.json")},
		}
		runPostToolUseHook(dir, input)

		dirty := readLines(filepath.Join(mark42Dir(dir), "dirty-files"))
		if len(dirty) != 0 {
			t.Errorf("excluded file should not create dirty files, got %d", len(dirty))
		}
	})
```

- [ ] **Step 5: Run tests to verify pass**

```bash
go test ./cmd/memory/... -run TestPostToolUseHook -v 2>&1 | tail -30
```

Expected: all subtests PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/memory/hook_post_tool_use.go cmd/memory/hook_post_tool_use_test.go
git commit -m "feat: PostToolUse writes events to SQLite, expands to all tools"
```

---

### Task 4: Stop Hook Completes Existing Session

**Files:**
- Modify: `cmd/memory/hook_stop.go`
- Modify: `cmd/memory/hook_stop_test.go`

- [ ] **Step 1: Write failing tests**

Add two new subtests to `TestHookStop` in `cmd/memory/hook_stop_test.go` (before the final closing `}`):

```go
	t.Run("completes existing session when current-session file present", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		store := newTestStore(t, t.TempDir()) // newTestStore defined in hook_memory_sync_test.go

		session, _ := store.CreateSession(filepath.Base(dir))
		os.WriteFile(filepath.Join(m42, "current-session"), []byte(session.Name), 0o644)
		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("main.go\n"), 0o644)

		runStopHook(dir, withStore(store))

		got, err := store.GetSession(session.Name)
		if err != nil {
			t.Fatalf("GetSession failed: %v", err)
		}
		if got.Status != "completed" {
			t.Errorf("session status = %q, want completed", got.Status)
		}
		if got.Summary == "" {
			t.Error("session should have a non-empty summary")
		}

		// current-session file should be deleted
		if _, err := os.Stat(filepath.Join(m42, "current-session")); !os.IsNotExist(err) {
			t.Error("current-session file should be deleted after stop")
		}
	})

	t.Run("creates fallback session when current-session file absent", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		store := newTestStore(t, t.TempDir()) // newTestStore defined in hook_memory_sync_test.go

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("main.go\n"), 0o644)
		// no current-session file written

		runStopHook(dir, withStore(store))

		sessions, _ := store.ListSessions(filepath.Base(dir), "completed", 10)
		if len(sessions) != 1 {
			t.Errorf("expected 1 completed fallback session, got %d", len(sessions))
		}
	})
```

Also add `storage` import to the test file:

```go
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)
```

- [ ] **Step 2: Run to verify failure**

```bash
go test ./cmd/memory/... -run TestHookStop/completes_existing -v 2>&1 | tail -10
```

Expected: FAIL — `runStopHook` doesn't use `withStore` or read current-session file.

- [ ] **Step 3: Rewrite `hook_stop.go`**

Replace the full contents of `cmd/memory/hook_stop.go` with:

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

type stopInput struct {
	TranscriptPath       string `json:"transcript_path"`
	LastAssistantMessage string `json:"last_assistant_message"`
	StopHookActive       bool   `json:"stop_hook_active"`
}

func withStopInput(input *stopInput) hookOption {
	return func(cfg *hookConfig) {
		cfg.stopInput = input
	}
}

var hookStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop hook: trigger memory sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		var input stopInput
		_ = readStdinJSON(&input)

		runStopHook(projectDir, withStopInput(&input))
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookStopCmd)
}

func runStopHook(projectDir string, opts ...hookOption) {
	if projectDir == "" {
		return
	}

	cfg := &hookConfig{}
	for _, o := range opts {
		o(cfg)
	}

	m42 := mark42Dir(projectDir)

	// Flag guard — only fire once per session
	if !touchFlag(filepath.Join(m42, "stop-prompted")) {
		return
	}

	projectName := filepath.Base(projectDir)

	// Read current session name (created at session-start)
	currentSessionFile := filepath.Join(m42, "current-session")
	sessionNameBytes, _ := os.ReadFile(currentSessionFile)
	sessionName := strings.TrimSpace(string(sessionNameBytes))

	// Read dirty files
	files := readLines(filepath.Join(m42, "dirty-files"))

	// Build and write session digest from transcript
	var lastMsg string
	if cfg.stopInput != nil {
		lastMsg = cfg.stopInput.LastAssistantMessage
		if cfg.stopInput.TranscriptPath != "" {
			digest := buildSessionDigest(cfg.stopInput.TranscriptPath)
			if digest != "" {
				_ = os.WriteFile(filepath.Join(m42, "session-digest.md"), []byte(digest), 0o644)
			}
		}
	}

	// Complete or create+complete session in SQLite (silent, no blocking)
	summary := buildAutoSummary(files, lastMsg)
	completeSession(projectName, sessionName, summary, cfg.store)

	// Cleanup
	clearFile(filepath.Join(m42, "dirty-files"))
	_ = os.Remove(currentSessionFile)

	// Sync CC auto-memory files into mark42 (silent, non-blocking)
	if memDir := ccMemoryDir(projectDir); memDir != "" {
		if syncStore, err := getStore(); err == nil {
			defer syncStore.Close()
			syncCCMemory(projectSlug(projectDir), memDir, syncStore, filepath.Join(m42, "memory-checksums.json"))
		} else {
			logger.Warn("failed to open store for cc memory sync", "err", err)
		}
	}

	// Stop hook: silent exit 0 = approve stop (no JSON output).
}

// completeSession completes an existing session or creates one as a fallback.
// If sessionName is non-empty, the session was created at session-start and we just finalize it.
// If sessionName is empty (SessionStart never ran), we create a fresh session.
func completeSession(projectName, sessionName, summary string, storeOverride *storage.Store) {
	store := storeOverride
	if store == nil {
		var err error
		store, err = getStore()
		if err != nil {
			return
		}
		defer store.Close()
	}

	if sessionName != "" {
		_ = store.CompleteSession(sessionName, summary)
		return
	}

	// Fallback: create and complete (SessionStart never ran)
	session, err := store.CreateSession(projectName)
	if err != nil {
		return
	}
	_ = store.CompleteSession(session.Name, summary)
}

func buildAutoSummary(files []string, lastMsg string) string {
	if len(files) == 0 && lastMsg == "" {
		return "Session with no tracked changes."
	}

	var parts []string

	if len(files) > 0 {
		names := make([]string, 0, len(files))
		for _, f := range files {
			name := filepath.Base(strings.SplitN(f, " [", 2)[0])
			names = append(names, name)
		}
		seen := map[string]bool{}
		unique := names[:0]
		for _, n := range names {
			if !seen[n] {
				seen[n] = true
				unique = append(unique, n)
			}
		}
		if len(unique) <= 5 {
			parts = append(parts, fmt.Sprintf("Modified %d files: %s", len(unique), strings.Join(unique, ", ")))
		} else {
			parts = append(parts, fmt.Sprintf("Modified %d files: %s, +%d more", len(unique), strings.Join(unique[:5], ", "), len(unique)-5))
		}
	}

	if lastMsg != "" {
		parts = append(parts, "Session context: "+truncate(lastMsg, maxContextLen))
	}

	if len(parts) == 0 {
		return "Session with no tracked changes."
	}
	return strings.Join(parts, ". ") + "."
}

const (
	maxMessageLen = 500
	maxDigestSize = 30 * 1024
	maxContextLen = 200
)

func buildSessionDigest(transcriptPath string) string {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var sb strings.Builder
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)

	for scanner.Scan() {
		if sb.Len() >= maxDigestSize {
			break
		}

		line := scanner.Bytes()
		var msg transcriptMessage
		if json.Unmarshal(line, &msg) != nil {
			continue
		}

		switch msg.Type {
		case "user":
			text := extractUserText(msg.Message)
			if text == "" {
				continue
			}
			text = truncate(text, maxMessageLen)
			sb.WriteString("### User\n")
			sb.WriteString(text)
			sb.WriteString("\n\n")

		case "assistant":
			text := extractAssistantText(msg.Message)
			if text == "" {
				continue
			}
			text = truncate(text, maxMessageLen)
			sb.WriteString("### Assistant\n")
			sb.WriteString(text)
			sb.WriteString("\n\n")
		}
	}

	if err := scanner.Err(); err != nil && sb.Len() == 0 {
		fmt.Fprintf(os.Stderr, "[mark42] digest scanner error: %v\n", err)
	}

	result := strings.TrimSpace(sb.String())
	if len(result) > maxDigestSize {
		result = result[:maxDigestSize]
	}
	return result
}

type transcriptMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type messageEnvelope struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func extractUserText(raw json.RawMessage) string {
	var env messageEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return ""
	}

	var s string
	if json.Unmarshal(env.Content, &s) == nil {
		return s
	}

	return ""
}

func extractAssistantText(raw json.RawMessage) string {
	var env messageEnvelope
	if json.Unmarshal(raw, &env) != nil {
		return ""
	}

	var blocks []contentBlock
	if json.Unmarshal(env.Content, &blocks) != nil {
		return ""
	}

	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

- [ ] **Step 4: Update stop hook tests that referenced session-events**

In `cmd/memory/hook_stop_test.go`:

**"silent output with events but no files"** — remove session-events setup and cleared check:
```go
	t.Run("silent output with events but no files", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte(""), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("stop hook should produce no output (silent approve), got: %s", buf.String())
		}
	})
```

**"clears both buffers after output"** — rename to "clears dirty-files after output", remove session-events check:
```go
	t.Run("clears dirty-files after output", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty)) != "" {
			t.Error("dirty-files should be cleared by hook")
		}
	})
```

**"handles 60+ events without error"** — remove session-events setup and assertion:
```go
	t.Run("handles many events without error", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"), []byte("a.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("stop hook should produce no output, got: %s", buf.String())
		}

		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty)) != "" {
			t.Error("dirty-files should be cleared")
		}
	})
```

**"silent output when files edited"** — remove session-events setup:
```go
	t.Run("silent output when files edited", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		os.WriteFile(filepath.Join(m42, "dirty-files"),
			[]byte("src/main.go\nsrc/lib.go\n"), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("stop hook should produce no output (silent approve), got: %s", buf.String())
		}

		dirty, _ := os.ReadFile(filepath.Join(m42, "dirty-files"))
		if strings.TrimSpace(string(dirty)) != "" {
			t.Error("dirty-files should be cleared")
		}
	})
```

**Update `buildAutoSummary` tests** — remove events arg from all three calls in `TestBuildAutoSummaryWithContext`:

```go
func TestBuildAutoSummaryWithContext(t *testing.T) {
	t.Run("enriches summary with last_assistant_message", func(t *testing.T) {
		files := []string{"main.go"}
		lastMsg := "Implemented the search feature with FTS5 ranking."

		summary := buildAutoSummary(files, lastMsg)
		if !strings.Contains(summary, "Session context:") {
			t.Errorf("summary should contain session context, got: %s", summary)
		}
		if !strings.Contains(summary, "Implemented the search") {
			t.Errorf("summary should contain last message content, got: %s", summary)
		}
	})

	t.Run("truncates long last_assistant_message at 200 chars", func(t *testing.T) {
		files := []string{"main.go"}
		longMsg := strings.Repeat("z", 300)

		summary := buildAutoSummary(files, longMsg)
		contextLine := ""
		for _, line := range strings.Split(summary, ". ") {
			if strings.HasPrefix(line, "Session context:") {
				contextLine = line
				break
			}
		}
		if len(contextLine) > 225 {
			t.Errorf("session context line too long (%d chars): %.100s...", len(contextLine), contextLine)
		}
		if strings.Contains(contextLine, strings.Repeat("z", 250)) {
			t.Error("long message should be truncated, but full content found")
		}
	})

	t.Run("skips context line when no last message", func(t *testing.T) {
		files := []string{"main.go"}

		summary := buildAutoSummary(files, "")
		if strings.Contains(summary, "Session context:") {
			t.Errorf("summary should not contain session context when empty, got: %s", summary)
		}
	})
}
```

Also update `TestStopHookWritesDigest` — remove session-events setup lines:

```go
func TestStopHookWritesDigest(t *testing.T) {
	t.Run("writes digest to mark42 dir", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		transcriptPath := filepath.Join(dir, "transcript.jsonl")
		line := `{"type":"user","message":{"role":"user","content":"Hello world"}}` + "\n"
		os.WriteFile(transcriptPath, []byte(line), 0o644)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf), withStopInput(&stopInput{
			TranscriptPath: transcriptPath,
		}))

		digestPath := filepath.Join(m42, "session-digest.md")
		data, err := os.ReadFile(digestPath)
		if err != nil {
			t.Fatalf("digest file not created: %v", err)
		}
		if !strings.Contains(string(data), "Hello world") {
			t.Error("digest file should contain transcript content")
		}
	})

	t.Run("skips digest when no transcript path", func(t *testing.T) {
		dir := setupProjectDir(t)
		m42 := mark42Dir(dir)

		var buf captureBuffer
		runStopHook(dir, withOutput(&buf))

		digestPath := filepath.Join(m42, "session-digest.md")
		_, err := os.Stat(digestPath)
		if err == nil {
			t.Error("digest file should not be created without transcript path")
		}
	})
}
```

- [ ] **Step 5: Run all stop tests**

```bash
go test ./cmd/memory/... -run "TestHookStop|TestBuildAutoSummary|TestStopHook|TestBuildSession" -v 2>&1 | tail -40
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/memory/hook_stop.go cmd/memory/hook_stop_test.go
git commit -m "feat: Stop hook reads current-session and completes SQLite session"
```

---

### Task 5: Full Test Suite + Build Verification

- [ ] **Step 1: Run full test suite with race detector**

```bash
go test -race ./... 2>&1 | tail -30
```

Expected: all tests pass, no data races. If any test fails, diagnose and fix before continuing.

- [ ] **Step 2: Build both binaries**

```bash
make build-all
```

Expected: exits 0, produces `mark42` and `mark42-server` binaries.

- [ ] **Step 3: Smoke test the hooks compile correctly**

```bash
./mark42 hook session-start --help
./mark42 hook post-tool-use --help
./mark42 hook stop --help
```

Expected: each prints usage without error.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: verify async hooks implementation compiles and tests pass"
```

- [ ] **Step 5: Push branch**

```bash
git push -u origin feat/async-hooks
```
