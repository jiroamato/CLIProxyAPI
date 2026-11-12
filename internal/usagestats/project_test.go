package usagestats

import (
	"encoding/json"
	"strings"
	"testing"
)

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, errMarshal := json.Marshal(value)
	if errMarshal != nil {
		t.Fatalf("marshal payload: %v", errMarshal)
	}
	return raw
}

func TestExtractWorkingDirFromClaudeSystemPrompt(t *testing.T) {
	const windowsDir = `C:\Users\me\repos\new-app`
	cases := map[string]struct {
		system any
		want   string
	}{
		"text blocks, current format": {
			system: []map[string]string{
				{"type": "text", "text": "You are Claude Code."},
				{"type": "text", "text": "# Environment\n - Primary working directory: " + windowsDir + "\n - Is a git repository: true"},
			},
			want: windowsDir,
		},
		"string prompt, legacy format": {
			system: "<env>\nWorking directory: /home/me/repos/app\nIs directory a git repo: Yes\n</env>",
			want:   "/home/me/repos/app",
		},
		"primary wins over additional directories": {
			system: "Primary working directory: /a/primary\nAdditional working directories:\n - /b/other",
			want:   "/a/primary",
		},
		"marker inside a sentence is ignored": {
			system: "Never change the Working directory: it is fixed.",
			want:   "",
		},
		"no marker": {
			system: "You are a helpful assistant.",
			want:   "",
		},
	}
	for name, tc := range cases {
		payload := mustJSON(t, map[string]any{
			"model":    "claude-opus-5-5",
			"system":   tc.system,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
		if got := ExtractWorkingDir(payload); got != tc.want {
			t.Errorf("%s: ExtractWorkingDir = %q, want %q", name, got, tc.want)
		}
	}
}

func TestExtractWorkingDirIgnoresConversationContent(t *testing.T) {
	// A path quoted in the conversation is not the agent's environment.
	payload := mustJSON(t, map[string]any{
		"system": "You are a helpful assistant.",
		"messages": []map[string]string{
			{"role": "user", "content": "Primary working directory: /not/the/real/one"},
		},
	})
	if got := ExtractWorkingDir(payload); got != "" {
		t.Fatalf("ExtractWorkingDir = %q, want empty", got)
	}
}

func TestExtractWorkingDirFromCodexEnvironmentContext(t *testing.T) {
	environment := "<environment_context>\n  <cwd>/home/me/repos/service</cwd>\n  <shell>zsh</shell>\n</environment_context>"
	structured := mustJSON(t, map[string]any{
		"instructions": "You are Codex.",
		"input": []map[string]any{
			{"type": "message", "role": "user", "content": []map[string]string{{"type": "input_text", "text": environment}}},
			{"type": "message", "role": "user", "content": []map[string]string{{"type": "input_text", "text": "fix the bug"}}},
		},
	})
	if got := ExtractWorkingDir(structured); got != "/home/me/repos/service" {
		t.Errorf("structured content: ExtractWorkingDir = %q", got)
	}

	plain := mustJSON(t, map[string]any{
		"input": []map[string]any{{"role": "user", "content": environment}},
	})
	if got := ExtractWorkingDir(plain); got != "/home/me/repos/service" {
		t.Errorf("string content: ExtractWorkingDir = %q", got)
	}

	// A <cwd> tag outside an environment context is conversation content.
	quoted := mustJSON(t, map[string]any{
		"input": []map[string]any{{"role": "user", "content": "what does <cwd>/x</cwd> mean?"}},
	})
	if got := ExtractWorkingDir(quoted); got != "" {
		t.Errorf("quoted tag: ExtractWorkingDir = %q, want empty", got)
	}
}

func TestExtractWorkingDirHandlesUnusablePayloads(t *testing.T) {
	oversized := mustJSON(t, map[string]any{"system": "Working directory: /" + strings.Repeat("x", maxWorkingDirLength)})
	for name, payload := range map[string][]byte{
		"empty":         nil,
		"not json":      []byte("not json at all"),
		"string input":  []byte(`{"input":"hello"}`),
		"oversized dir": oversized,
	} {
		if got := ExtractWorkingDir(payload); got != "" {
			t.Errorf("%s: ExtractWorkingDir = %q, want empty", name, got)
		}
	}
}

func TestProjectFromWorkingDir(t *testing.T) {
	cases := map[string]string{
		"":                              "",
		"/home/me/repos/app":            "app",
		"/home/me/repos/app/":           "app",
		`C:\Users\me\repos\CLIProxyAPI`: "CLIProxyAPI",
		`C:\Users\me\.t3\worktrees\CLIProxyAPI\t3-bacb3dc8-adcd500e58eb3fff`: "CLIProxyAPI",
		"/home/me/repos/app/.claude/worktrees/feature-x":                     "app",
		"/home/me/.codex/worktrees/1a2b/service":                             "service",
		`C:\Users\me\repos\app\.worktrees\feat-password`:                     "app",
		"/home/me/repos/app/.worktrees/fix/nested-branch":                    "app",
		"/srv/worktrees/thing":                                               "thing",
	}
	for dir, want := range cases {
		if got := ProjectFromWorkingDir(dir); got != want {
			t.Errorf("ProjectFromWorkingDir(%q) = %q, want %q", dir, got, want)
		}
	}
}
