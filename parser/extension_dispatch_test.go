package parser_test

import (
	"context"
	"testing"

	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/extension"
	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
)

type syntaxExtension struct{}

func (syntaxExtension) Register(registry *extension.Registry) error {
	if err := registry.Register(extension.Registration{
		ID: "note", Phase: extension.BlockPhase, Priority: 10, Triggers: []byte{':'}, Handler: noteBlockParser{},
	}); err != nil {
		return err
	}
	return registry.Register(extension.Registration{
		ID: "mention", Phase: extension.InlinePhase, Priority: 10, Triggers: []byte{'@'}, Handler: mentionInlineParser{},
	})
}

type noteBlockParser struct{}

func (noteBlockParser) ParseBlock(current extension.ParseContext) (extension.Match, bool, error) {
	const marker = ":::note"
	start := current.Offset()
	if start+len(marker) > len(current.Source()) || string(current.Source()[start:start+len(marker)]) != marker {
		return extension.Match{}, false, nil
	}
	node := current.Builder().Add(ast.Custom, ast.Span{Start: start, End: start + len(marker)})
	current.Builder().SetCustom(node, "note", nil)
	return extension.Match{Node: node, Consumed: len(marker)}, true, nil
}

type mentionInlineParser struct{}

func (mentionInlineParser) ParseInline(current extension.ParseContext) (extension.Match, bool, error) {
	start := current.Offset()
	end := start + 1
	for end < len(current.Source()) && current.Source()[end] >= 'a' && current.Source()[end] <= 'z' {
		end++
	}
	if end == start+1 {
		return extension.Match{}, false, nil
	}
	node := current.Builder().Add(ast.Custom, ast.Span{Start: start, End: end})
	current.Builder().SetCustom(node, "mention", string(current.Source()[start+1:end]))
	return extension.Match{Node: node, Consumed: end - start}, true, nil
}

func TestSyntaxExtensionsDispatchByPhaseAndTrigger(t *testing.T) {
	t.Parallel()
	p, err := parser.New(profile.CommonMark0312, parser.WithExtensions(syntaxExtension{}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Parse(context.Background(), []byte(":::note\n- hello @gaon\n"))
	if err != nil {
		t.Fatal(err)
	}
	var customKinds []string
	if err := result.Document.Walk(result.Document.Root(), func(node ast.Node, entering bool) error {
		if entering && node.Kind() == ast.Custom {
			customKinds = append(customKinds, node.CustomKind())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(customKinds) != 2 || customKinds[0] != "note" || customKinds[1] != "mention" {
		t.Fatalf("custom nodes = %v", customKinds)
	}
}

type crossingInlineExtension struct{}

func (crossingInlineExtension) Register(registry *extension.Registry) error {
	return registry.Register(extension.Registration{
		ID: "crossing", Phase: extension.InlinePhase, Priority: 10, Triggers: []byte{'@'}, Handler: crossingInlineParser{},
	})
}

type crossingInlineParser struct{}

func (crossingInlineParser) ParseInline(current extension.ParseContext) (extension.Match, bool, error) {
	start := current.Offset()
	node := current.Builder().Add(ast.Custom, ast.Span{Start: start, End: start + 4})
	current.Builder().SetCustom(node, "crossing", nil)
	return extension.Match{Node: node, Consumed: 4}, true, nil
}

func TestInlineExtensionRejectsConsumptionAcrossContainerSourceGaps(t *testing.T) {
	t.Parallel()
	p, err := parser.New(profile.CommonMark0312, parser.WithExtensions(crossingInlineExtension{}))
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Parse(context.Background(), []byte("> @a\n> b\n"))
	if err == nil || err.Error() != "parser: inline extension crossing crossed a non-contiguous source span" {
		t.Fatalf("error = %v", err)
	}
}
