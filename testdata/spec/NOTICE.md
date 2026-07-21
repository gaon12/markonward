# Specification fixture notice

The files in this directory are test data, not part of Markonward's MIT-licensed
source code.

- `commonmark-0.31.2.json` is derived from CommonMark 0.31.2, © John MacFarlane
  and contributors, licensed under [CC BY-SA 4.0]. Its canonical source is
  <https://spec.commonmark.org/0.31.2/>.
- `gfm-0.29.0.gfm.0.txt` is the GitHub Flavored Markdown 0.29 specification,
  © John MacFarlane and contributors, licensed under [CC BY-SA 4.0]. It is
  pinned to the `0.29.0.gfm.0` tag of `github/cmark-gfm`.

Run `go run ./internal/specsync` from the repository root to re-download both
fixtures. The command verifies their pinned SHA-256 digests before replacing
the local copies.

[CC BY-SA 4.0]: https://creativecommons.org/licenses/by-sa/4.0/
