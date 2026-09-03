package distill

import (
	"errors"
	"sort"
	"strconv"
	"strings"

	"github.com/mfenderov/mark42/internal/storage"
)

var ErrNothingToDistill = errors.New("nothing to distill")

type Summarizer interface {
	Summarize(extract Extract) string
}

type Extract struct {
	EventCount int
	Files      []string
	Commands   []string
	Tools      []string
}

type StructuralSummarizer struct{}

func (StructuralSummarizer) Summarize(e Extract) string {
	var sb strings.Builder
	sb.WriteString("Session: ")
	sb.WriteString(strconv.Itoa(e.EventCount))
	sb.WriteString(" tool events")
	if len(e.Files) > 0 {
		sb.WriteString(", ")
		sb.WriteString(strconv.Itoa(len(e.Files)))
		sb.WriteString(" files touched (")
		sb.WriteString(strings.Join(e.Files, ", "))
		sb.WriteString(")")
	}
	if len(e.Commands) > 0 {
		sb.WriteString(", commands: ")
		sb.WriteString(strings.Join(e.Commands, ", "))
	}
	if len(e.Tools) > 0 {
		sb.WriteString(", tools: ")
		sb.WriteString(strings.Join(e.Tools, ", "))
	}
	return sb.String()
}

func ExtractFromEvents(events []storage.SessionEvent) Extract {
	fileCount := make(map[string]int)
	cmdCount := make(map[string]int)
	toolSet := make(map[string]bool)

	for _, evt := range events {
		if evt.ToolName != "" {
			toolSet[evt.ToolName] = true
		}
		if evt.FilePath != "" {
			fileCount[evt.FilePath]++
		}
		if evt.Command != "" {
			cmdCount[evt.Command]++
		}
	}

	extract := Extract{EventCount: len(events)}
	extract.Files = orderByFrequency(fileCount)
	extract.Commands = orderByFrequency(cmdCount)
	if len(extract.Commands) > 10 {
		extract.Commands = extract.Commands[:10]
	}

	extract.Tools = make([]string, 0, len(toolSet))
	for t := range toolSet {
		extract.Tools = append(extract.Tools, t)
	}
	sort.Strings(extract.Tools)

	return extract
}

func orderByFrequency(counts map[string]int) []string {
	type entry struct {
		value string
		count int
	}
	entries := make([]entry, 0, len(counts))
	for v, c := range counts {
		entries = append(entries, entry{v, c})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].value < entries[j].value
	})
	result := make([]string, len(entries))
	for i, e := range entries {
		result[i] = e.value
	}
	return result
}

func Run(store *storage.Store, sessionName string, s Summarizer) error {
	events, err := store.GetSessionEvents(sessionName)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return ErrNothingToDistill
	}
	extract := ExtractFromEvents(events)
	summary := s.Summarize(extract)
	if err := store.UpdateSessionSummary(sessionName, summary); err != nil {
		return err
	}
	return store.DeleteSessionEvents(sessionName)
}
