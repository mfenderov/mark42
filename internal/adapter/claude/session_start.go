package claude

import (
	"path/filepath"
	"strings"

	"github.com/mfenderov/mark42/internal/state"
	"github.com/mfenderov/mark42/internal/storage"
)

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

	// Session recall
	results, err := store.GetRecentSessionSummaries(projectName, 72, 500)
	if err == nil && len(results) > 0 {
		formatted := storage.FormatSessionRecall(results)
		if formatted != "" {
			parts = append(parts, strings.TrimSpace(formatted))
		}
	}

	// Knowledge graph context
	ctxCfg := storage.DefaultContextConfig()
	ctxCfg.TokenBudget = 1500
	ctxResults, err := store.GetContextForInjection(ctxCfg, projectName, projectName, nil)
	if err == nil && len(ctxResults) > 0 {
		formatted := storage.FormatContextResults(ctxResults)
		if formatted != "" {
			parts = append(parts, strings.TrimSpace(formatted))
		}
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
