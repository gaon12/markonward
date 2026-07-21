# EnhanceMark v1 rules

[한국어](enhancemark.ko.md)

EnhanceMark v1 is modern GFM plus narrowly defined intent rules. None of these
rules run in `CommonMark0312`, `GFM029`, or `GFM`.

## Single tilde

`~~text~~` always keeps GFM strikethrough behavior. A single tilde between two
range operands is literal and takes precedence over single-tilde strikethrough.
An operand is currently a Unicode letter or digit, including the registered
Korean/date/time/unit characters `年 月 日 時 分 秒 개 명 번 회 층 장 권 차 주 월
일 시 분 초`.

| Input | Interpretation |
| --- | --- |
| `서울~부산` | literal range separator |
| `9시~18시` | literal range separator |
| `1~3명` | literal range separator |
| `~취소선~` | strikethrough when both delimiters flank content |
| `~~취소선~~` | GFM strikethrough |

When range and strikethrough readings are both plausible, the range wins and
`enhance.inline.tilde.range` is recorded with decision `literal`.

## Paired-punctuation emphasis

CommonMark flanking is evaluated first. If it rejects an `*` or `**` opener,
EnhanceMark may accept it only when a valid closer exists and the entire inner
content starts and ends with a registered pair:

| Opening | Closing | Opening | Closing |
| --- | --- | --- | --- |
| `"` | `"` | `'` | `'` |
| `(` | `)` | `[` | `]` |
| `{` | `}` | `“` | `”` |
| `‘` | `’` | `「` | `」` |
| `『` | `』` | `《` | `》` |
| `〈` | `〉` | `【` | `】` |
| `（` | `）` | `［` | `］` |
| `｛` | `｝` | | |

Thus `문장**"강조"**` can produce a `Strong` node after the CommonMark opener
was rejected. `<` and `>` are intentionally absent because they conflict with
HTML and autolink recognition. A successful override records
`enhance.inline.emphasis.paired-punctuation`.

## Incomplete inline recovery

Policies are configured per AST kind:

- `Literal`: keep the unmatched marker as text.
- `RecoverAtParagraphEnd`: create the formatting node through the paragraph end,
  emit diagnostic `enhance.unclosed-inline`, and record a recovered trace event.
- `Error`: abort parsing with the byte offset.

EnhanceMark defaults emphasis, strong, and strikethrough to paragraph-end
recovery. Code spans support all three policies when explicitly configured.
Links and images support only `Literal` and `Error`, because an absent target
cannot be inferred safely. Empty constructs are not recovered.

Recovery is applied recursively while parsing the inline range. Nested unmatched
markers therefore close from the inside out as the recursive calls return. The
normalized Markdown renderer always writes explicit closing markers, so a
recovered document does not require recovery on its second parse.

## Stability boundary

Rule IDs, diagnostics, pair tables, and operand tables are part of the
`EnhanceMarkV1` contract once v1 is released. Before that release, changes must
be covered by bilingual trace golden tests and profile-difference tests. Broad
natural-language guessing is deliberately out of scope: an ambiguous construct
that does not meet a listed rule remains literal.
