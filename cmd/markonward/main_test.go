package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gaon12/markonward/trace"
)

func runCLI(t *testing.T, input string, arguments ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), arguments, strings.NewReader(input), &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func TestConvertDefaultsToEnhanceHTML(t *testing.T) {
	t.Parallel()
	code, stdout, stderr := runCLI(t, "문장**\"강조\"**", "convert")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if stdout != "<p>문장<strong>&quot;강조&quot;</strong></p>\n" {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestExplainSupportsKoreanAndJSONLines(t *testing.T) {
	t.Parallel()
	code, korean, stderr := runCLI(t, "**unfinished", "explain", "--locale", "ko")
	if code != 0 || stderr != "" || !strings.Contains(korean, "문단 끝에서 복구") {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, korean, stderr)
	}
	code, jsonLines, stderr := runCLI(t, "**ok**", "explain", "--format", "jsonl")
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	decoder := json.NewDecoder(strings.NewReader(jsonLines))
	var event trace.Event
	if err := decoder.Decode(&event); err != nil || event.SchemaVersion != trace.SchemaVersion {
		t.Fatalf("event=%#v err=%v output=%q", event, err, jsonLines)
	}
}

func TestFlagsMayFollowInputFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "input.md")
	if err := os.WriteFile(path, []byte("*text*"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI(t, "", "convert", path, "--to", "text", "--from", "commonmark")
	if code != 0 || stderr != "" || stdout != "text\n" {
		t.Fatalf("code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestInvalidUTF8ReturnsFailure(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	code := execute(context.Background(), []string{"convert"}, bytes.NewReader([]byte{0xff}), &stdout, &stderr)
	if code != 1 || !strings.Contains(stderr.String(), "valid UTF-8") {
		t.Fatalf("code=%d stderr=%q", code, stderr.String())
	}
}
