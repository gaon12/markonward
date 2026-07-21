package html_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	markhtml "github.com/gaon12/markonward/renderer/html"
)

func render(t *testing.T, selected profile.Profile, source string, options ...markhtml.Option) string {
	t.Helper()
	p, err := parser.New(selected)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Parse(context.Background(), []byte(source))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := markhtml.New(options...).Render(context.Background(), &output, result.Document); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestHTMLRendersCoreAndGFMNodes(t *testing.T) {
	t.Parallel()
	got := render(t, profile.GFM, "# Hi\n\n- [x] **done**\n\na | b\n---|:---:\n1 | 2\n")
	for _, want := range []string{"<h1>Hi</h1>", `<input disabled="" type="checkbox" checked="">`, "<strong>done</strong>", "<table>", `<th align="center">`} {
		if !strings.Contains(got, want) {
			t.Fatalf("HTML missing %q:\n%s", want, got)
		}
	}
}

func TestSafeHTMLBlocksRawContentAndDangerousURLs(t *testing.T) {
	t.Parallel()
	got := render(t, profile.GFM, `<script>alert(1)</script>`+"\n\n"+`[click](javascript:alert(1))`)
	if strings.Contains(got, "<script>") || strings.Contains(got, `href="javascript:`) {
		t.Fatalf("unsafe output: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") || !strings.Contains(got, `href=""`) {
		t.Fatalf("safe transformations missing: %s", got)
	}
}

func TestUnsafeGFMStillAppliesTagFilter(t *testing.T) {
	t.Parallel()
	got := render(t, profile.GFM, `<script>alert(1)</script>`, markhtml.WithUnsafe())
	if !strings.Contains(got, "&lt;script>") {
		t.Fatalf("GFM tagfilter missing: %s", got)
	}
}

func TestOrderedListStartAttributeIsInsideOpeningTag(t *testing.T) {
	t.Parallel()
	got := render(t, profile.CommonMark0312, "2. second\n")
	if got != "<ol start=\"2\">\n<li>second</li>\n</ol>\n" {
		t.Fatalf("ordered-list HTML = %q", got)
	}
}
