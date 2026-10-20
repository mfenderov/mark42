package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mfenderov/mark42/internal/storage"
)

func TestPrintEntity(t *testing.T) {
	var buf bytes.Buffer
	old := out
	out = &buf
	defer func() { out = old }()

	printEntity(&storage.Entity{Name: "TestEntity", Type: "pattern", Observations: []string{"obs one", "obs two"}})

	got := buf.String()
	for _, want := range []string{"TestEntity", "pattern", "obs one", "obs two"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q: %q", want, got)
		}
	}

	buf.Reset()
	printEntity(&storage.Entity{Name: "Empty", Type: "concept"})
	if got := buf.String(); !strings.Contains(got, "Empty") || strings.Contains(got, "•") {
		t.Errorf("expected name only, no bullets: %q", got)
	}
}
