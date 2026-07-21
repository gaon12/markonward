package markonward_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gaon12/markonward"
	"github.com/gaon12/markonward/ast"
	"github.com/gaon12/markonward/extension"
	"github.com/gaon12/markonward/profile"
	markhtml "github.com/gaon12/markonward/renderer/html"
	markmarkdown "github.com/gaon12/markonward/renderer/markdown"
	marktext "github.com/gaon12/markonward/renderer/plaintext"
)

func TestEngineComposesExplicitComponents(t *testing.T) {
	t.Parallel()
	engine, err := markonward.New(profile.EnhanceMarkV1, markhtml.New())
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	result, err := engine.Convert(context.Background(), &output, []byte("**unfinished"))
	if err != nil {
		t.Fatal(err)
	}
	if output.String() != "<p><strong>unfinished</strong></p>\n" || len(result.Diagnostics) != 1 {
		t.Fatalf("output=%q diagnostics=%#v", output.String(), result.Diagnostics)
	}
}

type customRenderExtension struct{}

func (customRenderExtension) Register(registry *extension.Registry) error {
	return registry.Register(extension.Registration{
		ID: "mark", Phase: extension.RenderPhase, Priority: 10, Handler: bracketRenderHandler{},
	})
}

type bracketRenderHandler struct{}

func (bracketRenderHandler) Render(current extension.RenderContext, id ast.NodeID, entering bool) error {
	if entering {
		if _, err := io.WriteString(current.Writer(), "["); err != nil {
			return err
		}
		return current.RenderChildren(id)
	}
	_, err := io.WriteString(current.Writer(), "]")
	return err
}

func TestRendererOnlyExtensionsHandleCustomNodes(t *testing.T) {
	t.Parallel()
	source := []byte("value")
	builder := ast.NewBuilder("custom", source, true)
	paragraph := builder.Add(ast.Paragraph, ast.Span{Start: 0, End: len(source)})
	custom := builder.Add(ast.Custom, ast.Span{Start: 0, End: len(source)})
	builder.SetCustom(custom, "mark", nil)
	text := builder.Add(ast.Text, ast.Span{Start: 0, End: len(source)})
	builder.AppendChild(custom, text)
	builder.AppendChild(paragraph, custom)
	builder.AppendChild(builder.Document().Root(), paragraph)
	document, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	htmlRenderer, err := markhtml.NewWithExtensions(customRenderExtension{})
	if err != nil {
		t.Fatal(err)
	}
	textRenderer, err := marktext.NewWithExtensions(customRenderExtension{})
	if err != nil {
		t.Fatal(err)
	}
	markdownRenderer, err := markmarkdown.NewWithExtensions(profile.CommonMark0312, customRenderExtension{})
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		renderer interface {
			Render(context.Context, io.Writer, *ast.Document) error
		}
		want string
	}{
		"html":     {renderer: htmlRenderer, want: "<p>[value]</p>\n"},
		"text":     {renderer: textRenderer, want: "[value]\n"},
		"markdown": {renderer: markdownRenderer, want: "[value]\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			if err := test.renderer.Render(context.Background(), &output, document); err != nil {
				t.Fatal(err)
			}
			if output.String() != test.want {
				t.Fatalf("output = %q, want %q", output.String(), test.want)
			}
		})
	}
}
