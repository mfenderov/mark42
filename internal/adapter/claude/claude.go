package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/log"

	"github.com/mfenderov/mark42/internal/paths"
	"github.com/mfenderov/mark42/internal/storage"
)

// HookInput is the PostToolUse hook stdin payload.
type HookInput struct {
	ToolName  string         `json:"tool_name"`
	ToolInput map[string]any `json:"tool_input"`
}

// StopInput is the Stop hook stdin payload.
type StopInput struct {
	TranscriptPath       string `json:"transcript_path"`
	LastAssistantMessage string `json:"last_assistant_message"`
	StopHookActive       bool   `json:"stop_hook_active"`
}

// PluginConfig is the mark42 plugin config.json.
type PluginConfig struct {
	TriggerMode string `json:"triggerMode"`
}

// ParsePostToolUseInput decodes the PostToolUse hook stdin JSON.
func ParsePostToolUseInput(r io.Reader) (HookInput, error) {
	var input HookInput
	err := json.NewDecoder(r).Decode(&input)
	return input, err
}

// ParseStopInput decodes the Stop hook stdin JSON.
func ParseStopInput(r io.Reader) (StopInput, error) {
	var input StopInput
	err := json.NewDecoder(r).Decode(&input)
	return input, err
}

// Option configures hook execution.
type Option func(*Config)

// Config holds hook execution dependencies.
type Config struct {
	writer    *CaptureBuffer
	stopInput *StopInput
	store     *storage.Store
}

// CaptureBuffer captures hook stdout for tests.
type CaptureBuffer struct {
	strings.Builder
}

// WithStore injects a store, bypassing StoreFactory.
func WithStore(s *storage.Store) Option {
	return func(cfg *Config) { cfg.store = s }
}

// WithOutput captures hook stdout into a buffer.
func WithOutput(buf *CaptureBuffer) Option {
	return func(cfg *Config) { cfg.writer = buf }
}

// WithStopInput injects a stop-input payload.
func WithStopInput(input *StopInput) Option {
	return func(cfg *Config) { cfg.stopInput = input }
}

func printHook(cfg *Config, a ...any) {
	if cfg.writer != nil {
		fmt.Fprintln(cfg.writer, a...)
	} else {
		fmt.Fprintln(os.Stdout, a...)
	}
}

func printfHook(cfg *Config, format string, a ...any) {
	if cfg.writer != nil {
		fmt.Fprintf(cfg.writer, format, a...)
	} else {
		fmt.Fprintf(os.Stdout, format, a...)
	}
}

// logger writes operational messages to stderr.
var logger = log.NewWithOptions(os.Stderr, log.Options{
	ReportTimestamp: false,
})

// StoreFactory opens the mark42 store. The CLI overrides this to honor --db.
var StoreFactory = defaultStoreFactory

func defaultStoreFactory() (*storage.Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dbPath := paths.ResolveDBPath(home)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	return storage.NewStore(dbPath)
}

func getOrUseStore(cfg *Config) (*storage.Store, bool) {
	if cfg.store != nil {
		return cfg.store, false
	}
	s, err := StoreFactory()
	if err != nil {
		return nil, false
	}
	return s, true
}
