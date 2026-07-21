# Architecture and ownership

[한국어](architecture.ko.md)

## Package boundaries

```text
profile ─┐
         ├─> parser ─> ast <─ renderer/html
trace ───┤              ├─── renderer/plaintext
diagnostic┘              └─── renderer/markdown

extension ─> parser syntax/transform pipeline + custom render handlers
markonward ─> optional parser + renderer composition
cmd/markonward ─> CLI only
```

The core module has no third-party runtime dependency. Renderer packages do not
feed back into the parser, so parser-only applications do not link them. The
top-level package is convenience composition, not a mandatory gateway.

## Document arena

Each document owns a hybrid arena. `NodeID` is a 32-bit index into a contiguous
node-record slice; zero is invalid. Kind, packed source/content spans, tree
links, flags, and small integer metadata live in that slice. Optional strings
and custom payloads live in a sparse map. This keeps ordinary text-heavy trees
compact without making custom nodes impossible.

Parent/first-child/last-child/previous-sibling/next-sibling links make tree
navigation constant-time. A `Builder` is mutable until `Build`, validates the
tree, then yields a read-only `Document` suitable for concurrent rendering.

Public `Span` values are zero-based UTF-8 byte ranges `[Start, End)`. Line and
Unicode code-point columns are indexed lazily on the first `Position` call.

## Source lifetime

| Entry point | Source owner | Rule |
| --- | --- | --- |
| `Parse(ctx, []byte)` | caller | do not mutate or reuse bytes while the document is alive |
| `ParseCopy(ctx, []byte)` | document | caller may immediately reuse its input |
| `ParseReader(ctx, io.Reader)` | document | reads through the configured size guard |
| `ast.NewBuilder(..., borrow)` | selected by caller | same borrow/own contract as above |

Parsing never modifies source. A default 64 MiB guard applies to parser input;
`WithMaxInputBytes` changes it. Invalid UTF-8, I/O failures, cancellation, size
limits, and trace-sink failures are fatal. Markdown recovery is represented by
diagnostics rather than a fatal error unless a rule's policy is `Error`.

## Pipeline

1. Validate context, input size, and UTF-8.
2. Scan source lines, dispatch matching block triggers, and create source-mapped block nodes.
3. Resolve references, dispatch matching inline triggers, and process delimiter runs sequentially.
4. Run registered AST transforms in deterministic priority order.
5. Validate and freeze the arena.
6. Renderers walk the immutable document independently.

One document is deliberately parsed sequentially so trace sequence numbers are
deterministic. An immutable `Parser` or renderer may process many documents in
parallel. Trace sinks are responsible for their own concurrency; built-in sinks
serialize writes.

## Extensions

`extension.Registry` rejects duplicate IDs and registrations with overlapping
triggers at the same phase and priority. There is no global mutable registry.
The API defines block, inline, transform, custom-node, and render contracts.
`parser.WithExtensions` freezes block, inline, and transform hooks when the
parser is constructed. Syntax handlers receive a restricted context and must
return a positive consumed length plus a valid node; block handlers must consume
complete source lines. Inline handlers see original-source offsets and cannot
consume across source gaps created by stripping container markers.
`html.NewWithExtensions`,
`plaintext.NewWithExtensions`, and `markdown.NewWithExtensions` compile render
handlers without importing the parser. A render registration ID is the custom
node kind it handles; the handler owns child traversal through `RenderChildren`.

## Complexity and limits

The normal block scan and tree walk are linear in input/nodes. Inline emphasis
uses a delimiter-run plan with fixed inline storage for ordinary paragraphs and
overflow storage for adversarial input. Fuzzing guards against panic,
non-termination, and invalid spans; the pinned CommonMark and GFM suites enforce
the complete built-in syntax contracts.
