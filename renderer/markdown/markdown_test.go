package markdown_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	markmarkdown "github.com/gaon12/markonward/renderer/markdown"
)

func normalize(t *testing.T, selected profile.Profile, source string) string {
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
	if err := markmarkdown.New(selected).Render(context.Background(), &output, result.Document); err != nil {
		t.Fatal(err)
	}
	return output.String()
}

func TestNormalizedMarkdownIsIdempotent(t *testing.T) {
	t.Parallel()
	source := "# title ###\n\n- [x] **done**\n\na|b\n---|:---:\n1|2\n\n````go\n```\n````\n"
	first := normalize(t, profile.GFM, source)
	second := normalize(t, profile.GFM, first)
	if second != first {
		t.Fatalf("normalization is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

func TestRecoveredEnhanceFormattingGetsExplicitCloser(t *testing.T) {
	t.Parallel()
	got := normalize(t, profile.EnhanceMarkV1, "**unfinished")
	if got != "**unfinished**\n" {
		t.Fatalf("normalized recovery = %q", got)
	}
}
