package cli_test

import (
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/cli"
)

func TestCLI_RememberRecallForget_Workflow(t *testing.T) {
	root := cli.NewRootCmd()
	buf, store := setupTestCLI(t)

	// 1. Remember
	buf.Reset()
	_, err := executeCommand(root, "remember", "cache-service", "Redis 7.0", "Eviction is allkeys-lru",
		"--type", "infrastructure", "--rel", "primary-db:caches", "--project", "my-project")
	if err != nil {
		t.Fatalf("remember command failed: %v", err)
	}
	if !strings.Contains(buf.String(), "cache-service") {
		t.Errorf("expected remember output to mention topic, got: %s", buf.String())
	}

	// Verify in store
	entity, err := store.GetEntity("cache-service")
	if err != nil {
		t.Fatalf("GetEntity failed: %v", err)
	}
	if len(entity.Observations) != 2 {
		t.Errorf("expected 2 observations, got %d", len(entity.Observations))
	}

	// 2. Recall by topic
	buf.Reset()
	_, err = executeCommand(root, "recall", "--topic", "cache-service")
	if err != nil {
		t.Fatalf("recall --topic failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Redis 7.0") || !strings.Contains(buf.String(), "primary-db") {
		t.Errorf("expected recall to show facts and relations, got: %s", buf.String())
	}

	// 3. Recall by query
	buf.Reset()
	_, err = executeCommand(root, "recall", "Redis")
	if err != nil {
		t.Fatalf("recall query failed: %v", err)
	}
	if !strings.Contains(buf.String(), "cache-service") {
		t.Errorf("expected recall query to find cache-service, got: %s", buf.String())
	}

	// 4. Recall default context
	buf.Reset()
	_, err = executeCommand(root, "recall", "--project", "my-project")
	if err != nil {
		t.Fatalf("recall default context failed: %v", err)
	}
	if buf.String() == "" {
		t.Errorf("expected non-empty context recall output")
	}

	// 5. Forget specific fact
	buf.Reset()
	_, err = executeCommand(root, "forget", "cache-service", "Redis 7.0")
	if err != nil {
		t.Fatalf("forget fact failed: %v", err)
	}
	if !strings.Contains(buf.String(), "Invalidated") && !strings.Contains(buf.String(), "cache-service") {
		t.Errorf("expected forget confirmation, got: %s", buf.String())
	}

	// Verify fact is forgotten
	buf.Reset()
	_, _ = executeCommand(root, "recall", "--topic", "cache-service")
	if strings.Contains(buf.String(), "Redis 7.0") {
		t.Errorf("expected forgotten fact to be omitted, got: %s", buf.String())
	}

	// 6. Forget entire topic permanently
	buf.Reset()
	_, err = executeCommand(root, "forget", "cache-service", "--permanent")
	if err != nil {
		t.Fatalf("forget topic permanently failed: %v", err)
	}

	_, err = store.GetEntity("cache-service")
	if err == nil {
		t.Errorf("expected entity to be deleted after permanent forget")
	}
}
