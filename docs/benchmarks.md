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
go test -C benchmarks -run '^$' -bench 'Benchmark(Parse|ParseHTML)$' \
  -benchmem -count 10 ./... > benchmarks/results/current.txt

go tool -modfile=tools/go.mod benchstat benchmarks/results/current.txt

go run ./internal/benchgate -input benchmarks/results/current.txt
```

PowerShell redirection may produce UTF-16; `benchgate` accepts UTF-8, UTF-16LE,
and UTF-16BE benchmark files.

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

The final 10-sample local run on 2026-07-21 produced parser geometric-mean
ratios of `0.746x ns/op`, `0.743x B/op`, and `0.304x allocs/op`; parse+HTML
produced `0.714x`, `0.520x`, and `0.537x`. The gate still failed because
parser-only `readme` measured `1.589x ns/op`, above the per-fixture ceiling.
The old two-core Windows host varied by more than 20x on identical workloads;
the previously failing `small` fixture moved to `0.778x ns/op`, 3072 B/op, and
13 allocs/op. A controlled 10-sample timing rerun remains required. Results are
not marketed as stable numbers, and no v1 tag may bypass the gate.
