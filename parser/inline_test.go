package parser_test

import (
	"context"
	"testing"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	"github.com/gaon12/markonward/trace"
)

func paragraphChildren(document *ast.Document) []ast.Node {
	block := document.Node(document.Root()).FirstChild()
	var result []ast.Node
	for child := document.Node(block).FirstChild(); child != ast.NoNode; child = document.Node(child).NextSibling() {
		result = append(result, document.Node(child))
	}
	return result
}

func TestCommonMarkInlineNodes(t *testing.T) {
	t.Parallel()
	document := parse(t, profile.CommonMark0312, "an *em* and **strong** with [link](https://example.com \"title\")")
	children := paragraphChildren(document)
	var emphasis, strong, link bool
	for _, child := range children {
		switch child.Kind() { //nolint:exhaustive // The test intentionally tracks only the requested node types.
		case ast.Emphasis:
			emphasis = true
		case ast.Strong:
			strong = true
		case ast.Link:
			link = child.Destination() == "https://example.com" && child.Title() == "title"
		}
	}
	if !emphasis || !strong || !link {
		t.Fatalf("inline kinds missing: emphasis=%v strong=%v link=%v; nodes=%v", emphasis, strong, link, children)
	}
}

func TestPairedPunctuationIsEnhanceOnly(t *testing.T) {
	t.Parallel()
	source := "문장**\"강조\"**"
	commonMark := paragraphChildren(parse(t, profile.CommonMark0312, source))
	for _, node := range commonMark {
		if node.Kind() == ast.Strong {
			t.Fatal("CommonMark unexpectedly applied paired-punctuation enhancement")
		}
	}
	enhance := paragraphChildren(parse(t, profile.EnhanceMarkV1, source))
	found := false
	for _, node := range enhance {
		found = found || node.Kind() == ast.Strong
	}
	if !found {
		t.Fatalf("EnhanceMark nodes = %v, want strong", enhance)
	}
}

func TestKoreanRangeAndStrikethrough(t *testing.T) {
	t.Parallel()
	document := parse(t, profile.EnhanceMarkV1, "서울~부산과 ~취소~ 및 ~~삭제~~")
	children := paragraphChildren(document)
	strikeCount := 0
	for _, node := range children {
		if node.Kind() == ast.Strikethrough {
			strikeCount++
		}
	}
	if strikeCount != 2 {
		t.Fatalf("strikethrough count = %d, nodes=%v", strikeCount, children)
	}
}

func TestEnhanceRecoversAtParagraphEnd(t *testing.T) {
	t.Parallel()
	collector := &trace.Collector{}
	p, err := parser.New(profile.EnhanceMarkV1, parser.WithTrace(collector))
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Parse(context.Background(), []byte("before **unfinished"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "enhance.unclosed-inline" {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	foundRecovery := false
	for _, event := range collector.Events() {
		foundRecovery = foundRecovery || event.Decision == trace.Recovered
	}
	if !foundRecovery {
		t.Fatal("trace has no recovery event")
	}
}

func TestParseCopyOwnsSource(t *testing.T) {
	t.Parallel()
	p, err := parser.New(profile.CommonMark0312)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.ParseCopy(context.Background(), []byte("text"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Document.BorrowsSource() {
		t.Fatal("ParseCopy document should own its source")
	}
}
