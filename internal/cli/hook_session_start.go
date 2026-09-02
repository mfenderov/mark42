package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

type hookOption func(*hookConfig)

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

type captureBuffer struct {
	strings.Builder
}

func withOutput(buf *captureBuffer) hookOption {
	return func(cfg *hookConfig) {
		cfg.writer = buf
	}
}

func hookPrint(cfg *hookConfig, a ...any) {
	if cfg.writer != nil {
		fmt.Fprintln(cfg.writer, a...)
	} else {
		fmt.Fprintln(out, a...)
	}
}

func hookPrintf(cfg *hookConfig, format string, a ...any) {
	if cfg.writer != nil {
		fmt.Fprintf(cfg.writer, format, a...)
	} else {
		fmt.Fprintf(out, format, a...)
	}
}

var hookSessionStartCmd = &cobra.Command{
	Use:   "session-start",
	Short: "SessionStart hook: inject context",
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := getProjectDir()
		if projectDir == "" {
			return nil
		}

		store, err := getStore()
		if err != nil {
			return nil
		}
		defer store.Close()
		_ = store.Migrate()

		runSessionStartHook(projectDir, store)
		return nil
	},
}

func init() {
	hookCmd.AddCommand(hookSessionStartCmd)
}

func runSessionStartHook(projectDir string, store *storage.Store, opts ...hookOption) {
	cfg := &hookConfig{}
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
		writeCurrentSession(projectDir, session.Name)
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

	hookPrintf(cfg, "=== mark42: %s ===\n", projectName)
	hookPrintf(cfg, "[%d estimated tokens]\n\n", estimatedTokens)
	hookPrint(cfg, combined)
}
