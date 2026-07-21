# Security model

[한국어](security.ko.md)

Markdown is untrusted input by default. Markonward's safe HTML renderer applies
two structural defenses before bytes reach the output writer.

## Default HTML behavior

- Raw inline and block HTML is escaped.
- URL schemes are parsed after trimming control/space characters.
- Relative URLs and `http`, `https`, `mailto`, `tel`, and `ftp` schemes are
  allowed. Every other explicit scheme, including `javascript`, `vbscript`,
  `file`, and `data`, is replaced with an empty destination.
- HTML text, attributes, titles, code, and alt text are escaped for their output
  contexts.

`html.WithUnsafe()` is for specification tests or input already trusted by the
embedding application. It allows raw HTML and dangerous schemes. Documents with
a GFM-derived profile still apply GFM tagfilter to `title`, `textarea`, `style`,
`xmp`, `iframe`, `noembed`, `noframes`, `script`, and `plaintext` tags.

Unsafe mode is not an HTML sanitizer. If an application needs a different trust
policy, keep safe mode enabled or sanitize after rendering with a policy-aware
component outside this dependency-free core.

## Resource controls

- Input must be valid UTF-8.
- Parser input defaults to a 64 MiB maximum and can be lowered with
  `WithMaxInputBytes`.
- `context.Context` cancellation is checked during block and inline work and
  during rendering.
- Fuzz targets cover blocks, delimiters, links, tables, all renderers, and
  normalization round trips.
- Arena spans and relationships are validated before a document is returned.

The current pre-v1 inline implementation is not yet proven linear for every
adversarial delimiter pattern. Treat input limits and request deadlines as part
of deployment policy even after fuzzing.

## Source ownership and data races

`Parse` deliberately borrows caller memory. Mutating its bytes while a document
is parsed or rendered is a caller data race and may corrupt output. Use
`ParseCopy` or `ParseReader` across ownership boundaries. Documents and immutable
parser/renderer configurations may otherwise be shared across goroutines.

## Trace privacy

Trace events contain byte spans, adjacent characters, destinations/kinds in
fields, and localized positional output. A sink can therefore expose fragments
of private Markdown. Apply the same access control, retention, and redaction
policy used for application logs. Leave tracing disabled on sensitive hot paths
unless the explanation is actually required.

## Reporting

Do not publish exploitable details in an ordinary issue. Use GitHub's private
vulnerability reporting for `gaon12/markonward` once it is enabled. Until then,
contact the repository owner privately before disclosure.
