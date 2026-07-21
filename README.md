# Markonward

[한국어](README.ko.md)

Markonward is a dependency-free Go toolkit for parsing Markdown into a public,
source-mapped AST and rendering that AST as safe HTML, structured plain text,
or normalized Markdown. Its fourth profile, EnhanceMark, adds conservative
intent recovery for Korean writing without changing CommonMark or GFM modes.

> **Development status:** this repository is a pre-v1 implementation snapshot,
> not yet a v1.0 release. The pinned suites pass all 652/652 CommonMark 0.31.2
> examples and all 671/671 active GFM 0.29 examples, including extension
> examples. The release workflow also requires the comparative performance
> gate, which must be revalidated before publishing `v1.0.0`.

## Why Markonward?

- Parser, public AST, and each renderer can be imported independently.
- Runtime code uses only the Go standard library.
- `Parse` borrows its input for low allocation; `ParseCopy` and `ParseReader`
  create owned source.
- Trace-off parsing does not build trace events. Trace-on parsing emits stable,
  ordered decisions in JSON Lines or localized text.
- EnhanceMark distinguishes Korean ranges such as `서울~부산`, `9시~18시`, and
  `1~3명` from intentional single-tilde strikethrough.
- Parser and renderer configurations are immutable and safe for concurrent use.

## Requirements and local use

Go 1.26 or newer is required.

```sh
go test ./...
go run ./cmd/markonward convert README.md --from enhance --to html
go run ./cmd/markonward explain README.md --profile enhance --locale ko
```

The module path is `github.com/gaon12/markonward`. Until the release gates pass,
use a commit rather than assuming a stable tagged API.

## Library API

The library requires an explicit profile:

```go
p, err := parser.New(
    profile.EnhanceMarkV1,
    parser.WithTrace(sink),
    parser.WithRecovery(ast.Strong, parser.RecoverAtParagraphEnd),
)
if err != nil {
    return err
}

result, err := p.Parse(ctx, source) // borrows source
if err != nil {
    return err
}

if err := html.New().Render(ctx, dst, result.Document); err != nil {
    return err
}
if err := plaintext.New().Render(ctx, dst, result.Document); err != nil {
    return err
}
return markdown.New(profile.EnhanceMarkV1).Render(ctx, dst, result.Document)
```

Renderer-only users can build a document with `ast.NewBuilder`. Parser-only
users do not pull renderer packages into their dependency graph.

`ast.Span` is a zero-based, half-open UTF-8 byte range. `Document.Position`
computes one-based line and Unicode code-point columns lazily.

## Profiles

| Profile | Base | GFM extensions | Enhance inference |
| --- | --- | --- | --- |
| `CommonMark0312` | CommonMark 0.31.2 | No | Never |
| `GFM029` | CommonMark 0.29 | tables, tasks, strike, autolinks, tagfilter | Never |
| `GFM` | CommonMark 0.31.2 | tables, tasks, strike, autolinks, tagfilter | Never |
| `EnhanceMarkV1` | modern GFM | All modern GFM features | Korean ranges, paired punctuation, paragraph-end recovery |

The profile names describe the intended contracts. The conformance numbers at
the top of this file remain the authoritative implementation status until all
official examples pass.

## CLI

```text
markonward convert [FILE] --from enhance|commonmark|gfm|gfm029 \
  --to html|text|markdown [-o FILE] [--unsafe-html]

markonward explain [FILE] --profile enhance|commonmark|gfm|gfm029 \
  --format text|jsonl --locale en|ko --level decisions|verbose
```

Omitting `FILE` uses standard input; omitting `-o` uses standard output. The CLI
defaults to EnhanceMark while the library has no default profile. Invalid UTF-8
is rejected explicitly.

## Documentation

- [Architecture and ownership](docs/architecture.md)
- [EnhanceMark v1 rules](docs/enhancemark.md)
- [Trace schema and rule IDs](docs/trace.md)
- [Security model](docs/security.md)
- [Benchmarks and release thresholds](docs/benchmarks.md)

Every document has a complete Korean counterpart linked at its top.

## Verification

```sh
./scripts/check.sh
MARKONWARD_STRICT_SPECS=1 go test -run 'TestOfficial' ./parser
go test -C benchmarks -run '^$' -bench 'Benchmark(Parse|ParseHTML)$' \
  -benchmem -count 10 ./... > benchmarks/results/current.txt
go run ./internal/benchgate -input benchmarks/results/current.txt
```

Specification fixtures are SHA-256 pinned and carry a separate CC BY-SA notice
under `testdata/spec`. The comparison dependency, goldmark v1.8.4, is isolated
in the nested `benchmarks` module and is not a runtime dependency.

## License

Markonward source code is licensed under the [MIT License](LICENSE). Upstream
specification fixtures retain their [separate notice](testdata/spec/NOTICE.md).
