package benchmarks

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gaon12/markonward/parser"
	"github.com/gaon12/markonward/profile"
	markhtml "github.com/gaon12/markonward/renderer/html"
	"github.com/yuin/goldmark"
	goldextension "github.com/yuin/goldmark/extension"
	goldtext "github.com/yuin/goldmark/text"
)

var fixtures = map[string][]byte{
	"small":      []byte("# Heading\n\nA paragraph with **strong**, *emphasis*, and [a link](https://example.com).\n"),
	"korean":     []byte("# 운영 시간\n\n서울~부산 노선은 오전 9시~오후 6시에 **\"운영\"**합니다.\n\n- [x] 첫 번째 작업\n- [ ] 두 번째 작업\n"),
	"table":      []byte("| name | value | status |\n|:-----|------:|:------:|\n| alpha | 123 | **ready** |\n| beta | 456 | ~~old~~ |\n"),
	"delimiters": []byte(strings.Repeat("before ***nested _content_*** after ~range~ and `code`\n", 64)),
	"readme":     []byte(strings.Repeat("## Section\n\nParagraph text with [documentation](https://example.com/docs), `code`, and **important details**.\n\n", 64)),
}

func BenchmarkParse(b *testing.B) {
	markParser, err := parser.New(profile.GFM)
	if err != nil {
		b.Fatal(err)
	}
	gold := goldmark.New(goldmark.WithExtensions(goldextension.GFM))
	for name, source := range fixtures {
		b.Run(name, func(b *testing.B) {
			b.Run("markonward", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for b.Loop() {
					if _, parseErr := markParser.Parse(context.Background(), source); parseErr != nil {
						b.Fatal(parseErr)
					}
				}
			})
			b.Run("goldmark", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for b.Loop() {
					gold.Parser().Parse(goldtext.NewReader(source))
				}
			})
		})
	}
}

func BenchmarkParseHTML(b *testing.B) {
	markParser, err := parser.New(profile.GFM)
	if err != nil {
		b.Fatal(err)
	}
	markRenderer := markhtml.New(markhtml.WithUnsafe())
	gold := goldmark.New(goldmark.WithExtensions(goldextension.GFM))
	for name, source := range fixtures {
		b.Run(name, func(b *testing.B) {
			b.Run("markonward", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for b.Loop() {
					result, parseErr := markParser.Parse(context.Background(), source)
					if parseErr != nil {
						b.Fatal(parseErr)
					}
					if renderErr := markRenderer.Render(context.Background(), io.Discard, result.Document); renderErr != nil {
						b.Fatal(renderErr)
					}
				}
			})
			b.Run("goldmark", func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(source)))
				for b.Loop() {
					if convertErr := gold.Convert(source, io.Discard); convertErr != nil {
						b.Fatal(convertErr)
					}
				}
			})
		})
	}
}
