package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestAnalyticsTuneCommand_DryRun(t *testing.T) {
	tmpDir := t.TempDir()
	oldDBPath := dbPath
	dbPath = filepath.Join(tmpDir, "test.db")
	defer func() { dbPath = oldDBPath }()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore: %v", err)
	}
	if _, err := store.CreateEntity("Alpha", "project", []string{"hello"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	store.Close()

	if err := analyticsTuneCmd.RunE(analyticsTuneCmd, nil); err != nil {
		t.Fatalf("analytics tune command: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "Tuning Recommendation") {
		t.Errorf("output missing header\n%s", got)
	}
	if !strings.Contains(got, "No changes applied. Re-run with --apply to persist.") {
		t.Errorf("output missing dry-run footer\n%s", got)
	}
	if !strings.Contains(got, "FrequencyWeight") {
		t.Errorf("expected FrequencyWeight recommendation line\n%s", got)
	}
	if !strings.Contains(got, "sparse") {
		t.Errorf("expected sparse-signal rationale\n%s", got)
	}
}

func TestAnalyticsTuneCommand_Apply(t *testing.T) {
	tmpDir := t.TempDir()
	oldDBPath := dbPath
	dbPath = filepath.Join(tmpDir, "test.db")
	defer func() { dbPath = oldDBPath }()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	defer func() {
		f := analyticsTuneCmd.Flags().Lookup("apply")
		_ = analyticsTuneCmd.Flags().Set("apply", f.DefValue)
		f.Changed = false
	}()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore: %v", err)
	}
	if err := store.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if _, err := store.CreateEntity("Alpha", "project", []string{"hello"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	rec, err := store.RecommendTuning()
	if err != nil {
		t.Fatalf("RecommendTuning: %v", err)
	}
	store.Close()

	if err := analyticsTuneCmd.Flags().Set("apply", "true"); err != nil {
		t.Fatalf("set apply flag: %v", err)
	}

	if err := analyticsTuneCmd.RunE(analyticsTuneCmd, nil); err != nil {
		t.Fatalf("analytics tune --apply: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "Applied. New config saved.") {
		t.Errorf("output missing Applied\n%s", got)
	}

	store2, err := getStore()
	if err != nil {
		t.Fatalf("getStore: %v", err)
	}
	defer store2.Close()
	gotCfg := store2.GetImportanceConfig()
	if gotCfg != rec.Suggested {
		t.Errorf("GetImportanceConfig = %+v, want %+v", gotCfg, rec.Suggested)
	}
}

func TestAnalyticsCommand(t *testing.T) {
	tmpDir := t.TempDir()
	oldDBPath := dbPath
	dbPath = filepath.Join(tmpDir, "test.db")
	defer func() { dbPath = oldDBPath }()

	var buf bytes.Buffer
	oldOut := out
	out = &buf
	defer func() { out = oldOut }()

	store, err := getStore()
	if err != nil {
		t.Fatalf("getStore: %v", err)
	}
	if _, err := store.CreateEntity("Alpha", "project", []string{"hello"}); err != nil {
		t.Fatalf("CreateEntity: %v", err)
	}
	store.Close()

	if err := analyticsCmd.RunE(analyticsCmd, nil); err != nil {
		t.Fatalf("analytics command: %v", err)
	}
	got := buf.String()
	for _, want := range []string{"Memory Analytics", "Entities: 1", "Decay Curve", "Top Memories"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\n%s", want, got)
		}
	}
}
