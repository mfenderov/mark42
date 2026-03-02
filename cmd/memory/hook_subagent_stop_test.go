package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHookSubagentStop(t *testing.T) {
	t.Run("outputs systemMessage with agent summary", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := subagentStopInput{
			AgentType:            "knowledge-extractor",
			LastAssistantMessage: "Created 2 entities: Transaction Pattern (pattern), Cart API (decision). Added 3 observations.",
		}

		var buf captureBuffer
		runSubagentStopHook(dir, input, withOutput(&buf))

		got := buf.String()
		if got == "" {
			t.Fatal("expected output")
		}

		var output map[string]any
		if err := json.Unmarshal([]byte(strings.TrimSpace(got)), &output); err != nil {
			t.Fatalf("output is not valid JSON: %v\ngot: %s", err, got)
		}

		msg, ok := output["systemMessage"].(string)
		if !ok {
			t.Fatal("systemMessage is not a string")
		}
		if !strings.Contains(msg, "knowledge-extractor") {
			t.Errorf("systemMessage should mention agent type, got: %s", msg)
		}
		if !strings.Contains(msg, "2 entities") {
			t.Errorf("systemMessage should include agent summary, got: %s", msg)
		}
	})

	t.Run("truncates long messages", func(t *testing.T) {
		dir := setupProjectDir(t)

		longMsg := strings.Repeat("a", 600)
		input := subagentStopInput{
			AgentType:            "knowledge-extractor",
			LastAssistantMessage: longMsg,
		}

		var buf captureBuffer
		runSubagentStopHook(dir, input, withOutput(&buf))

		var output map[string]any
		json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &output)
		msg := output["systemMessage"].(string)

		if len(msg) > 600 {
			t.Errorf("systemMessage should be truncated, got %d chars", len(msg))
		}
	})

	t.Run("no output without project dir", func(t *testing.T) {
		input := subagentStopInput{
			AgentType:            "knowledge-extractor",
			LastAssistantMessage: "some result",
		}

		var buf captureBuffer
		runSubagentStopHook("", input, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("expected empty output, got: %s", buf.String())
		}
	})

	t.Run("no output for non-knowledge-extractor agents", func(t *testing.T) {
		dir := setupProjectDir(t)

		input := subagentStopInput{
			AgentType:            "some-other-agent",
			LastAssistantMessage: "some result",
		}

		var buf captureBuffer
		runSubagentStopHook(dir, input, withOutput(&buf))

		if buf.String() != "" {
			t.Errorf("should ignore non-knowledge-extractor agents, got: %s", buf.String())
		}
	})
}
