package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHookPreCompact(t *testing.T) {
	t.Run("outputs systemMessage with file count", func(t *testing.T) {
		dir := setupProjectDir(t)
		os.WriteFile(filepath.Join(mark42Dir(dir), "dirty-files"),
			[]byte("a.go\nb.go\nc.go\n"), 0o644)

		var buf captureBuffer
		runPreCompactHook(dir, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Fatal("expected output")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &output); err != nil {
			t.Fatalf("output not valid JSON: %v\ngot: %s", err, got)
		}

		msg, ok := output["systemMessage"].(string)
		if !ok {
			t.Fatal("missing systemMessage")
		}
		if !strings.Contains(msg, "3") {
			t.Errorf("systemMessage should contain file count, got: %s", msg)
		}
	})

	t.Run("zero files produces zero count in message", func(t *testing.T) {
		dir := setupProjectDir(t)

		var buf captureBuffer
		runPreCompactHook(dir, withOutput(&buf))

		var output map[string]any
		json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &output)

		msg, ok := output["systemMessage"].(string)
		if !ok {
			t.Fatal("missing systemMessage")
		}
		if !strings.Contains(msg, "0") {
			t.Errorf("systemMessage should contain 0, got: %s", msg)
		}
	})
}
