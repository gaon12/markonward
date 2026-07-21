# Benchmarks and release thresholds

[한국어](benchmarks.ko.md)

The comparison lives in a nested module so goldmark v1.8.4 never becomes a
Markonward runtime dependency. It compares modern GFM with tracing disabled.

## Corpora

| Fixture | Purpose |
| --- | --- |
| `small` | short heading, paragraph, emphasis, and link |
| `korean` | Korean ranges, paired punctuation, and tasks |
| `table` | aligned GFM table and inline formatting |
| `readme` | repeated documentation-shaped sections |
| `delimiters` | nested and adversarial delimiter pressure |

Every fixture runs as parser-only and parse+HTML. The latter uses trusted HTML
mode to match goldmark's conversion surface rather than measuring sanitization.

## Reproduce

Use an otherwise idle machine with fixed power/thermal policy. Ten samples are
required by the gate.

```sh
mkdir -p benchmarks/results
sh ./scripts/benchmark.sh benchmarks/results/current.txt 10

go tool -C tools benchstat ../benchmarks/results/current.txt

go run ./internal/benchgate -input benchmarks/results/current.txt
```

PowerShell redirection may produce UTF-16; `benchgate` accepts UTF-8, UTF-16LE,
and UTF-16BE benchmark files.

On Windows, `./scripts/benchmark.ps1` produces the same result. Each sample is
an independent `go test -count 1` invocation so the two implementations are
measured next to each other instead of letting long-term host drift bias ten
consecutive measurements of one implementation.

## v1 release gate

For each of `BenchmarkParse` and `BenchmarkParseHTML` independently:

1. Compute each implementation's geometric mean per fixture and metric.
2. Require every Markonward fixture ratio to be at most `1.15x` goldmark for
   `ns/op`, `B/op`, and `allocs/op`.
3. Compute the geometric mean of the five fixture ratios.
4. Require Markonward to be strictly below `1.0x` for all three metrics.

`internal/benchgate` implements those rules and rejects missing pairs or fewer
than ten samples. Shared GitHub runners are useful for regression artifacts but
have noisy timings; the final release decision should also be reproduced on a
controlled host.

## Current snapshot

The paired 10-sample local run on 2026-07-21 passed the release gate. Parser
geometric-mean ratios were `0.747x ns/op`, `0.742x B/op`, and `0.302x
allocs/op`; parse+HTML produced `0.622x`, `0.520x`, and `0.537x`. After indexing
overflow emphasis pairs, delimiter-heavy ratios were `0.794x` for parser-only
and `0.618x` for parse+HTML. The old two-core Windows host remains noisy, so
these are gate evidence rather than marketing numbers; release CI reruns the
same paired method and no v1 tag may bypass it.
