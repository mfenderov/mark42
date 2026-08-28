package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/storage"
)

var (
	dbPath  string
	Version = "dev"

	// logger writes operational messages (errors, info) to stderr
	logger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
	})

	// out is the destination for command output (search results, stats, etc.)
	out io.Writer = os.Stdout
)

// output writes command results to stdout (not stderr).
// This follows Unix conventions: data to stdout, logs to stderr.
func output(a ...any) {
	fmt.Fprintln(out, a...)
}

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("212"))

	entityStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("86"))

	typeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	obsStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252"))

	relationStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("219"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("78"))

	dimStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))
)

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

// NewRootCmd returns the assembled root command (exposed for testing).
func NewRootCmd() *cobra.Command {
	return rootCmd
}

var rootCmd = &cobra.Command{
	Use:   "mark42",
	Short: "Local memory system for Claude Code",
	Long: titleStyle.Render("mark42") + " - A privacy-first, SQLite-based memory system\n\n" +
		"Store entities, observations, and relations in a local database\n" +
		"with full-text search capabilities.",
}

func init() {
	defaultDB := filepath.Join(os.Getenv("HOME"), ".claude", "memory.db")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", defaultDB, "path to database file")

	rootCmd.AddCommand(entityCmd)
	rootCmd.AddCommand(obsCmd)
	rootCmd.AddCommand(relCmd)
	rootCmd.AddCommand(searchCmd)
	rootCmd.AddCommand(graphCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(statsCmd)
	rootCmd.AddCommand(versionCmd)
}

func getStore() (*storage.Store, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return storage.NewStore(dbPath)
}
