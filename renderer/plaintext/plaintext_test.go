package plaintext_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	"github.com/gaon12/markonward/renderer/plaintext"
)

func TestPlainTextPreservesUsefulStructure(t *testing.T) {
	t.Parallel()
	p, err := parser.New(profile.GFM)
	if err != nil {
		t.Fatal(err)
	}
	result, err := p.Parse(context.Background(), []byte("- [x] [done](https://example.com)\n\na | b\n---|---\n1 | 2\n"))
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := plaintext.New().Render(context.Background(), &output, result.Document); err != nil {
		t.Fatal(err)
	}
	got := output.String()
	for _, want := range []string{"[x] done (https://example.com)", "a\tb", "1\t2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain text missing %q:\n%s", want, got)
		}
	}
}
