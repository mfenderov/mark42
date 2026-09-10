package cli_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mfenderov/mark42/internal/cli"
)

func executeCommand(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err := root.Execute()
	return buf.String(), err
}

func TestPathSlugCmd(t *testing.T) {
	root := cli.NewRootCmd()

	output, err := executeCommand(root, "path", "slug", "/Users/mark/dev/myproject")
	if err != nil {
		t.Fatalf("path slug failed: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	expected := "-Users-mark-dev-myproject"
	if trimmed != expected {
		t.Errorf("expected slug %q, got %q", expected, trimmed)
	}
}

func TestPathStateDirCmd(t *testing.T) {
	root := cli.NewRootCmd()

	output, err := executeCommand(root, "path", "state-dir", "/Users/mark/dev/myproject")
	if err != nil {
		t.Fatalf("path state-dir failed: %v", err)
	}

	trimmed := strings.TrimSpace(output)
	if !strings.HasSuffix(trimmed, filepath.Join(".mark42", "state", "-Users-mark-dev-myproject")) {
		t.Errorf("unexpected state-dir output: %q", trimmed)
	}
}
