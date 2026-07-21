# Markonward benchmarks

This nested module keeps the goldmark comparison dependency out of the public
Markonward module. It compares equivalent modern-GFM parser-only and end-to-end
HTML workloads with tracing disabled.

Run ten samples on an otherwise idle machine:

```sh
go test -run '^$' -bench 'Benchmark(Parse|ParseHTML)$' -benchmem -count 10 ./... > current.txt
```

Compare saved runs with `benchstat`. Release evaluation uses geometric means for
`ns/op`, `B/op`, and `allocs/op`; GitHub-hosted runner timing is informational
because shared machines do not provide stable performance isolation.
