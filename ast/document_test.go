package ast_test

import (
	"reflect"
	"testing"

	"github.com/gaon12/markonward/ast"
)

func TestBuilderCreatesSourceAwareTree(t *testing.T) {
	t.Parallel()
	source := []byte("제목\n본문")
	builder := ast.NewBuilder("enhancemark-v1", source, true)
	heading := builder.Add(ast.Heading, ast.Span{Start: 0, End: 6})
	builder.SetIntegers(heading, 1, 0)
	text := builder.Add(ast.Text, ast.Span{Start: 0, End: 6})
	builder.AppendChild(heading, text)
	builder.AppendChild(builder.Document().Root(), heading)
	document, err := builder.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !document.BorrowsSource() {
		t.Fatal("document should borrow its input")
	}
	if got := document.Node(text).Text(); got != "제목" {
		t.Fatalf("Text() = %q", got)
	}
	if got := document.Position(7); !reflect.DeepEqual(got, ast.Position{Offset: 7, Line: 2, Column: 1}) {
		t.Fatalf("Position(7) = %#v", got)
	}
}

func TestWalkUsesDocumentOrder(t *testing.T) {
	t.Parallel()
	builder := ast.NewBuilder("test", []byte("ab"), true)
	paragraph := builder.Add(ast.Paragraph, ast.Span{Start: 0, End: 2})
	first := builder.Add(ast.Text, ast.Span{Start: 0, End: 1})
	second := builder.Add(ast.Text, ast.Span{Start: 1, End: 2})
	builder.AppendChild(paragraph, first)
	builder.AppendChild(paragraph, second)
	builder.AppendChild(builder.Document().Root(), paragraph)
	document, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	var visits []string
	if err := document.Walk(document.Root(), func(node ast.Node, entering bool) error {
		visits = append(visits, node.Kind().String()+map[bool]string{true: "+", false: "-"}[entering])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	want := []string{"document+", "paragraph+", "text+", "text-", "text+", "text-", "paragraph-", "document-"}
	if !reflect.DeepEqual(visits, want) {
		t.Fatalf("visits = %v, want %v", visits, want)
	}
}
