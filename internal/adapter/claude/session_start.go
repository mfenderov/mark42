package claude

import (
	"path/filepath"
	"strings"

	"github.com/mfenderov/mark42/internal/state"
	"github.com/mfenderov/mark42/internal/storage"
)

// recallSection returns the formatted recent-sessions recall, or "" when none.
func recallSection(store *storage.Store, projectName string) string {
	results, err := store.GetRecentSessionSummaries(projectName, 72, 500)
	if err != nil || len(results) == 0 {
		return ""
	}
	return strings.TrimSpace(storage.FormatSessionRecall(results))
}

// graphContextSection returns the formatted knowledge-graph context, or "" when none.
func graphContextSection(store *storage.Store, projectName string) string {
	ctxCfg := storage.DefaultContextConfig()
	ctxCfg.TokenBudget = 1500
	results, err := store.GetContextForInjection(ctxCfg, projectName, projectName, nil)
	if err != nil || len(results) == 0 {
		return ""
	}
	return strings.TrimSpace(storage.FormatContextResults(results))
}

// SessionStart runs the SessionStart hook: inject recall + knowledge graph context.
func SessionStart(projectDir string, store *storage.Store, opts ...Option) {
	cfg := &Config{}
	for _, o := range opts {
		o(cfg)
	}

	clearFlag(filepath.Join(mark42Dir(projectDir), "stop-prompted"))

	if store == nil {
		return
	}

	projectName := filepath.Base(projectDir)

	// Create pending session and record its name for PostToolUse and Stop hooks
	session, err := store.CreateSession(projectName)
	if err == nil {
		state.WriteCurrentSession(projectDir, session.Name)
	}

	var parts []string
	if s := recallSection(store, projectName); s != "" {
		parts = append(parts, s)
	}
	if s := graphContextSection(store, projectName); s != "" {
		parts = append(parts, s)
	}

	if len(parts) == 0 {
		return
	}

	combined := strings.ToValidUTF8(strings.Join(parts, "\n\n"), "")
	estimatedTokens := storage.EstimateTokens(combined)

	printfHook(cfg, "=== mark42: %s ===\n", projectName)
	printfHook(cfg, "[%d estimated tokens]\n\n", estimatedTokens)
	printHook(cfg, combined)
}
