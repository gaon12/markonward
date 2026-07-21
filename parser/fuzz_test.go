package parser_test

import (
	"bytes"
	"context"
	"testing"
	"unicode/utf8"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	markhtml "github.com/gaon12/markonward/renderer/html"
	markmarkdown "github.com/gaon12/markonward/renderer/markdown"
)

func FuzzParseProfiles(f *testing.F) {
	for _, seed := range []string{
		"# heading\n\nparagraph",
		"***nested*** and [link](https://example.com)",
		"서울~부산과 **\"강조\"**",
		"a | b\n---|:---:\n1 | 2",
		"````\n```\n````",
	} {
		f.Add([]byte(seed))
	}
	profiles := []profile.Profile{profile.CommonMark0312, profile.GFM029, profile.GFM, profile.EnhanceMarkV1}
	f.Fuzz(func(t *testing.T, source []byte) {
		if !utf8.Valid(source) || len(source) > 1<<20 {
			t.Skip()
		}
		for _, selected := range profiles {
			p, err := parser.New(selected)
			if err != nil {
				t.Fatal(err)
			}
			result, err := p.Parse(context.Background(), source)
			if err != nil {
				t.Fatalf("profile %s: %v", selected.ID(), err)
			}
			if err := result.Document.Validate(); err != nil {
				t.Fatalf("profile %s AST: %v", selected.ID(), err)
			}
		}
	})
}

func FuzzParseRenderRoundTrip(f *testing.F) {
	for _, seed := range []string{"plain", "**strong**", "[x](https://example.com)", "<em>raw</em>", "**unfinished"} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		if !utf8.Valid(source) || len(source) > 1<<20 {
			t.Skip()
		}
		p, err := parser.New(profile.EnhanceMarkV1)
		if err != nil {
			t.Fatal(err)
		}
		result, err := p.Parse(context.Background(), source)
		if err != nil {
			t.Fatal(err)
		}
		var htmlOutput, markdownOutput bytes.Buffer
		if err := markhtml.New().Render(context.Background(), &htmlOutput, result.Document); err != nil {
			t.Fatal(err)
		}
		if err := markmarkdown.New(profile.EnhanceMarkV1).Render(context.Background(), &markdownOutput, result.Document); err != nil {
			t.Fatal(err)
		}
		second, err := p.Parse(context.Background(), markdownOutput.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		var normalizedAgain bytes.Buffer
		if err := markmarkdown.New(profile.EnhanceMarkV1).Render(context.Background(), &normalizedAgain, second.Document); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(markdownOutput.Bytes(), normalizedAgain.Bytes()) {
			t.Fatalf("normalization changed on second pass:\nfirst=%q\nsecond=%q", markdownOutput.Bytes(), normalizedAgain.Bytes())
		}
	})
}
