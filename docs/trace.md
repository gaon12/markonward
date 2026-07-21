# Trace schema and rule IDs

[한국어](trace.ko.md)

Tracing explains deterministic parser decisions; it is not a debug dump of Go
implementation details. Without a sink, the parser does not construct trace-only
fields or retain events.

## Event schema v1

| Field | Meaning |
| --- | --- |
| `schema_version` | currently `1` |
| `sequence` | one-based order within a parse |
| `level` | `decisions` or `verbose` numeric enum in JSON |
| `phase` | `block`, `inline`, `transform`, or `render` |
| `rule_id` | stable language-neutral identifier |
| `decision` | `observed`, `accepted`, `rejected`, `literal`, or `recovered` |
| `span` | zero-based UTF-8 byte half-open range |
| `left`, `right` | optional adjacent Unicode characters |
| `node_kind` | optional created/recovered AST kind |
| `fields` | ordered name/value metadata |

JSON Lines is the stable machine format. Each `trace.Event` is one JSON object
followed by a newline. Text output is localized presentation; it displays
`[line:column @byte]` using one-based Unicode code-point positions.

## Levels

- `decisions` records semantic overrides, literal-vs-syntax choices, recovery,
  and other decisions important to an author.
- `verbose` additionally records candidate delimiters and CommonMark rejections.

## Rule catalog

| Rule ID | Level | Purpose |
| --- | --- | --- |
| `inline.delimiter.found` | verbose | observed a delimiter run |
| `commonmark.inline.emphasis.flanking` | verbose | accepted/rejected CommonMark flanking context |
| `enhance.inline.emphasis.paired-punctuation` | decisions | EnhanceMark overrode a rejected opener using a registered pair |
| `enhance.inline.tilde.range` | decisions | kept a single tilde as a range separator |
| `enhance.inline.recovery.paragraph-end` | decisions | recovered an unclosed inline node |
| `inline.node.created` | decisions | created an inline AST node |

## Example

```sh
printf '문장**"강조"**\n' | \
  markonward explain --profile enhance --format text --locale ko --level verbose
```

Use `--format jsonl` for logs, editor integrations, or regression fixtures.
Sink errors abort parsing and are returned to the caller. Built-in collector and
writer sinks are safe for concurrent calls, although each document's events are
still emitted in sequential parse order.
