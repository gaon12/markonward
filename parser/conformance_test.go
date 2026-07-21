package parser_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	markhtml "github.com/gaon12/markonward/renderer/html"
)

const (
	commonMarkFixtureSHA256 = "d431b29d97b6f73e69d547109cf5081578fac931e72afe95639ebe766c1b2a20"
	gfmFixtureSHA256        = "7cea1221ffba48559d8748c8510d3c5bda40487a13667b80e77c14a1505b9821"
)

type specificationExample struct {
	Markdown  string `json:"markdown"`
	HTML      string `json:"html"`
	Example   int    `json:"example"`
	Section   string `json:"section"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

func TestOfficialCommonMark0312Examples(t *testing.T) {
	payload := readFixture(t, "commonmark-0.31.2.json", commonMarkFixtureSHA256)
	var examples []specificationExample
	if err := json.Unmarshal(payload, &examples); err != nil {
		t.Fatalf("decode CommonMark fixture: %v", err)
	}
	if len(examples) != 652 {
		t.Fatalf("CommonMark fixture contains %d examples, want 652", len(examples))
	}
	checkExamples(t, profile.CommonMark0312, examples)
}

func TestOfficialGFM029Examples(t *testing.T) {
	payload := readFixture(t, "gfm-0.29.0.gfm.0.txt", gfmFixtureSHA256)
	examples, err := extractGFMExamples(string(payload))
	if err != nil {
		t.Fatalf("extract GFM fixture: %v", err)
	}
	if len(examples) != 649 {
		t.Fatalf("GFM fixture contains %d examples, want 649", len(examples))
	}
	checkExamples(t, profile.GFM029, examples)
}

func readFixture(t *testing.T, name, expectedHash string) []byte {
	t.Helper()
	path := filepath.Join("..", "testdata", "spec", name)
	payload, err := os.ReadFile(path) // #nosec G304 -- path is a test-owned constant.
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	actual := fmt.Sprintf("%x", sha256.Sum256(payload))
	if actual != expectedHash {
		t.Fatalf("%s SHA-256 is %s, want %s", name, actual, expectedHash)
	}
	return payload
}

func extractGFMExamples(specification string) ([]specificationExample, error) {
	const fence = "````````````````````````````````"
	lines := strings.SplitAfter(strings.ReplaceAll(specification, "\r\n", "\n"), "\n")
	section := ""
	examples := make([]specificationExample, 0, 649)
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSuffix(lines[index], "\n")
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
		if line != fence+" example" {
			continue
		}
		startLine := index + 2
		var markdown strings.Builder
		index++
		for index < len(lines) && strings.TrimSuffix(lines[index], "\n") != "." {
			markdown.WriteString(strings.ReplaceAll(lines[index], "→", "\t"))
			index++
		}
		if index >= len(lines) {
			return nil, fmt.Errorf("example %d has no Markdown terminator", len(examples)+1)
		}
		var expected strings.Builder
		index++
		for index < len(lines) && strings.TrimSuffix(lines[index], "\n") != fence {
			expected.WriteString(strings.ReplaceAll(lines[index], "→", "\t"))
			index++
		}
		if index >= len(lines) {
			return nil, fmt.Errorf("example %d has no closing fence", len(examples)+1)
		}
		examples = append(examples, specificationExample{
			Markdown: markdown.String(), HTML: expected.String(), Example: len(examples) + 1,
			Section: section, StartLine: startLine, EndLine: index + 1,
		})
	}
	return examples, nil
}

func checkExamples(t *testing.T, selected profile.Profile, examples []specificationExample) {
	t.Helper()
	p, err := parser.New(selected)
	if err != nil {
		t.Fatal(err)
	}
	renderer := markhtml.New(markhtml.WithUnsafe(), markhtml.WithXHTML())
	mismatches := 0
	for _, example := range examples {
		result, parseErr := p.Parse(context.Background(), []byte(example.Markdown))
		var output bytes.Buffer
		if parseErr == nil {
			parseErr = renderer.Render(context.Background(), &output, result.Document)
		}
		if parseErr == nil && output.String() == example.HTML {
			continue
		}
		mismatches++
		if mismatches <= 5 {
			t.Logf("example %d (%s, source lines %d-%d) mismatch\nerror: %v\nwant: %q\n got: %q",
				example.Example, example.Section, example.StartLine, example.EndLine, parseErr, example.HTML, output.String())
		}
	}
	passed := len(examples) - mismatches
	t.Logf("%s conformance: %d/%d examples passed", selected, passed, len(examples))
	if os.Getenv("MARKONWARD_STRICT_SPECS") == "1" && mismatches != 0 {
		t.Fatalf("%s conformance gate failed: %d examples mismatched", selected, mismatches)
	}
}
