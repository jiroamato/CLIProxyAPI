package usagestats

import (
	"strings"

	"github.com/MoeForge/moego/ojson"
)

const (
	// maxWorkingDirLength bounds the stored working directory.
	maxWorkingDirLength = 512
	// codexContextScanItems bounds how many leading input items are inspected.
	// Codex emits its environment context at the start of the conversation, and
	// scanning a long history on every request would be wasted work.
	codexContextScanItems = 8
)

// claudeWorkingDirMarkers are the system prompt line prefixes Claude Code uses
// to announce its working directory, newest first.
var claudeWorkingDirMarkers = []string{"Primary working directory: ", "Working directory: "}

// ExtractWorkingDir returns the working directory a coding agent declared in
// its request payload, or "" when the payload declares none.
//
// It only reads the fields where agents state their environment (the Anthropic
// "system" prompt and the Responses "input" messages), so a path that merely
// appears in conversation content is never mistaken for the working directory.
func ExtractWorkingDir(payload []byte) string {
	if len(payload) == 0 {
		return ""
	}
	// NoCopy avoids duplicating multi-megabyte payloads; cleanWorkingDir clones
	// the result so it never pins the payload buffer.
	if dir := claudeWorkingDir(ojson.GetBytesNoCopy(payload, "system")); dir != "" {
		return dir
	}
	return codexWorkingDir(ojson.GetBytesNoCopy(payload, "input"))
}

// claudeWorkingDir reads the working directory from an Anthropic system prompt,
// which is either a string or an array of text blocks.
func claudeWorkingDir(system ojson.Result) string {
	if !system.Exists() {
		return ""
	}
	if !system.IsArray() {
		return workingDirFromLine(system.String())
	}
	found := ""
	system.ForEach(func(_, block ojson.Result) bool {
		found = workingDirFromLine(block.Get("text").String())
		return found == ""
	})
	return found
}

func workingDirFromLine(text string) string {
	for _, marker := range claudeWorkingDirMarkers {
		start := strings.Index(text, marker)
		if start < 0 {
			continue
		}
		// Require the marker to start a line (optionally as a "- " list item) so
		// "Primary working directory: " is not also matched as "Working directory: ".
		prefix := strings.TrimRight(text[:start], " \t-")
		if prefix != "" && !strings.HasSuffix(prefix, "\n") {
			continue
		}
		rest := text[start+len(marker):]
		if end := strings.IndexAny(rest, "\r\n"); end >= 0 {
			rest = rest[:end]
		}
		if dir := cleanWorkingDir(rest); dir != "" {
			return dir
		}
	}
	return ""
}

// codexWorkingDir reads the <cwd> tag Codex places in its environment context
// message at the start of the Responses input list.
func codexWorkingDir(input ojson.Result) string {
	if !input.IsArray() {
		return ""
	}
	found := ""
	scanned := 0
	input.ForEach(func(_, item ojson.Result) bool {
		if scanned++; scanned > codexContextScanItems {
			return false
		}
		content := item.Get("content")
		if content.IsArray() {
			content.ForEach(func(_, part ojson.Result) bool {
				found = workingDirFromCwdTag(part.Get("text").String())
				return found == ""
			})
		} else {
			found = workingDirFromCwdTag(content.String())
		}
		return found == ""
	})
	return found
}

func workingDirFromCwdTag(text string) string {
	if !strings.Contains(text, "<environment_context>") {
		return ""
	}
	start := strings.Index(text, "<cwd>")
	if start < 0 {
		return ""
	}
	rest := text[start+len("<cwd>"):]
	end := strings.Index(rest, "</cwd>")
	if end < 0 {
		return ""
	}
	return cleanWorkingDir(rest[:end])
}

func cleanWorkingDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if len(dir) > maxWorkingDirLength {
		return ""
	}
	return strings.Clone(dir)
}

// ProjectFromWorkingDir derives a project name from a working directory: the
// repository directory name, with agent worktree checkouts folded back into
// the repository they belong to.
func ProjectFromWorkingDir(dir string) string {
	dir = strings.TrimRight(strings.ReplaceAll(strings.TrimSpace(dir), "\\", "/"), "/")
	if dir == "" {
		return ""
	}
	segments := strings.Split(dir, "/")
	for i, segment := range segments {
		if i == 0 {
			continue
		}
		if segment == ".worktrees" {
			// <repo>/.worktrees/<name>
			if segments[i-1] != "" {
				return segments[i-1]
			}
			continue
		}
		if segment != "worktrees" {
			continue
		}
		switch segments[i-1] {
		case ".claude":
			// <repo>/.claude/worktrees/<name>
			if i >= 2 && segments[i-2] != "" {
				return segments[i-2]
			}
		case ".t3":
			// ~/.t3/worktrees/<repo>/<id>
			if i+1 < len(segments) {
				return segments[i+1]
			}
		case ".codex":
			// ~/.codex/worktrees/<id>/<repo>
			if i+2 < len(segments) {
				return segments[i+2]
			}
		}
	}
	return segments[len(segments)-1]
}
