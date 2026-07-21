package parser_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
)

func parse(t *testing.T, selected profile.Profile, source string) *ast.Document {
	t.Helper()
	p, err := parser.New(selected)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Parse(context.Background(), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	return result.Document
}

func childKinds(document *ast.Document, parent ast.NodeID) []ast.Kind {
	var result []ast.Kind
	for child := document.Node(parent).FirstChild(); child != ast.NoNode; child = document.Node(child).NextSibling() {
		result = append(result, document.Node(child).Kind())
	}
	return result
}

func TestBlockStructure(t *testing.T) {
	t.Parallel()
	source := "# Heading\n\n> quote\n\n- first\n- [x] done\n\n```go\nfmt.Println()\n```\n"
	document := parse(t, profile.GFM, source)
	want := []ast.Kind{ast.Heading, ast.BlockQuote, ast.List, ast.CodeBlock}
	got := childKinds(document, document.Root())
	if len(got) != len(want) {
		t.Fatalf("root children = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("root children = %v, want %v", got, want)
		}
	}
}

func TestGFMTableIsProfileGated(t *testing.T) {
	t.Parallel()
	source := "a | b\n---|:---:\n1 | 2\n"
	gfm := parse(t, profile.GFM, source)
	if got := gfm.Node(gfm.Node(gfm.Root()).FirstChild()).Kind(); got != ast.Table {
		t.Fatalf("GFM first node = %s", got)
	}
	commonMark := parse(t, profile.CommonMark0312, source)
	if got := commonMark.Node(commonMark.Node(commonMark.Root()).FirstChild()).Kind(); got != ast.Paragraph {
		t.Fatalf("CommonMark first node = %s", got)
	}
}

func TestInputValidationAndOwnership(t *testing.T) {
	t.Parallel()
	p, err := parser.New(profile.CommonMark0312, parser.WithMaxInputBytes(4))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.Parse(context.Background(), []byte{0xff}); !errors.Is(err, parser.ErrInvalidUTF8) {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}
	if _, err := p.ParseReader(context.Background(), strings.NewReader("12345")); !errors.Is(err, parser.ErrInputTooLarge) {
		t.Fatalf("large input error = %v", err)
	}
}
