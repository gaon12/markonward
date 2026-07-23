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
	"github.com/gaon12/markonward/renderer/plaintext"
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
	seeds := []string{
		"plain",
		"**strong**",
		"[x](https://example.com)",
		"<em>raw</em>",
		"**unfinished",
		"` `00",
		`0\)`,
		"_0*0",
		" http://!",
		"0*0**!",
		"~!~00",
		"*!*0*0",
		"*0\f",
		"*0 \f",
		"0\r ",
		" *!*0**00",
		"**0*0",
		"0***0",
		"**0****0",
		"0\x00*0",
		"**!*000*\x04",
		"**\x00*0",
		"0*0 *0*\x00",
		"******!",
		"\x00***0",
		"0@.0",
		"*!***0**0",
		"\x00*0**0",
		"_!____*!",
		"****\x00*0",
		"\x00* \x00*",
		"http://>",
		"*0 *!*0**0",
		"****0*0",
		"*\n+ 0000",
		"0*0*****0",
		"~*~0",
		"**0**\x00*0",
		"*0**!*0**!",
		"_0_*_!0",
		"****0*\x00",
		"**_0*0",
		"_*_*00",
		"***0 *0**0",
		"__*_ * *!_*!       ***00000",
		"*****!*0 *!",
		"*******! *0",
		"****0*0*",
		"0****0",
		"*0****!",
		"***0****0",
		"0*0****!",
		"*0*\x00*0",
		"*!*0***0*",
		"+ * *",
		"*0****0**",
		"**\x00*0*0",
		"*\x00****0",
		"**\x00*0*",
		"***0****0*0",
		"***0**\x00",
		"***0**\x000",
		"****0**\x00*0",
		"00**0****0**",
		"0*_\x00*",
		"0**\x00*0",
		"http://>!",
		"* *0*_*0*",
		"**_*0*",
		"<0.@a.",
		"_*0*0",
		"***\x000***_0_",
		"****0**0**00",
		"***0****0**",
		"~0*0*~0",
		"_0*0**0*0",
		"****!0*000***",
		"*0*****0*****0",
		"**\x00*0*0**0",
		"\x00***0**0",
		".@0.",
		"****0***0*\x00",
		"**\x00*0****0",
		"*0** \x00*0",
		"* ```",
		"0**\x00*0*0",
		"***0*_*0*",
		"\x000***0**0",
		"0\x00***0*00**",
		"*0***\x00*0",
		"~~0 ~0~0",
		"*!_0_**!",
		" 0\x00!0",
		"**_0_*0*",
		"*__0__**0**\x00",
		"*__\x00_**0\x00_",
		"*!*00*0*\x00*0",
		"***0******0*",
		"****0*0****0*0",
		"*!*0**_\x00",
		"*0***_0_",
		"*0**0****0*",
		"***0******0**0",
		"0*\x00~0",
		"___0_*_0_*",
		"*0**!*0**!*",
		"*__&}}}}\x00_ 00\x00_",
		"***0******!!0*",
		"**0*0*0*!0*000",
		"*!**0****0*",
		"*!_!_____0_____ ",
		"*0***_!_",
		"[](\\ )",
		"_*_0_*_",
		"_\x00**0",
		"**0****0****\x00**",
		"[](\\()",
		"0\n0\n=",
		"0~~\x00~0",
		"*!***0*******!00*",
		"******0***0****!",
		"*0**!****0**0**",
	}
	for _, seed := range seeds {
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

func FuzzBlockSyntax(f *testing.F) {
	fuzzFocusedParser(f, []string{"# heading\n", "> quote\n", "- item\n  - nested\n", "```go\ncode\n```\n"})
}

func FuzzDelimiterSyntax(f *testing.F) {
	fuzzFocusedParser(f, []string{"***nested***", "_a **b** c_", "~~strike~~", "**\"강조\"**"})
}

func FuzzLinkSyntax(f *testing.F) {
	fuzzFocusedParser(f, []string{"[text](https://example.com)", "![alt](image.png \"title\")", "[ref]\n\n[ref]: /url"})
}

func FuzzTableSyntax(f *testing.F) {
	fuzzFocusedParser(f, []string{"a | b\n---|---\n1|2\n", "| a\\|b | c |\n|:---|---:|\n"})
}

func FuzzRendererSyntax(f *testing.F) {
	for _, seed := range []string{"plain", "<script>alert(1)</script>", "[x](javascript:alert(1))", "**unfinished"} {
		f.Add([]byte(seed))
	}
	p, err := parser.New(profile.EnhanceMarkV1)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		if !utf8.Valid(source) || len(source) > 1<<20 {
			t.Skip()
		}
		result, parseErr := p.Parse(context.Background(), source)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		var htmlOutput, markdownOutput, textOutput bytes.Buffer
		if renderErr := markhtml.New().Render(context.Background(), &htmlOutput, result.Document); renderErr != nil {
			t.Fatal(renderErr)
		}
		if renderErr := markmarkdown.New(profile.EnhanceMarkV1).Render(context.Background(), &markdownOutput, result.Document); renderErr != nil {
			t.Fatal(renderErr)
		}
		if renderErr := plaintext.New().Render(context.Background(), &textOutput, result.Document); renderErr != nil {
			t.Fatal(renderErr)
		}
	})
}

func fuzzFocusedParser(f *testing.F, seeds []string) {
	for _, seed := range seeds {
		f.Add([]byte(seed))
	}
	p, err := parser.New(profile.EnhanceMarkV1)
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, source []byte) {
		if !utf8.Valid(source) || len(source) > 1<<20 {
			t.Skip()
		}
		result, parseErr := p.Parse(context.Background(), source)
		if parseErr != nil {
			t.Fatal(parseErr)
		}
		if validationErr := result.Document.Validate(); validationErr != nil {
			t.Fatal(validationErr)
		}
	})
}
