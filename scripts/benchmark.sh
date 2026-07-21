#!/usr/bin/env sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output=${1:-"$root/benchmarks/results/current.txt"}
samples=${2:-10}

case "$samples" in
    ''|*[!0-9]*|0)
        echo "benchmark: samples must be a positive integer" >&2
        exit 2
        ;;
esac

mkdir -p "$(dirname -- "$output")"
: >"$output"

sample=1
while [ "$sample" -le "$samples" ]; do
    echo "# benchmark sample $sample/$samples"
    result=$(cd "$root" && go test -C benchmarks -run '^$' -bench 'Benchmark(Parse|ParseHTML)$' -benchmem -count 1 ./...)
    printf '%s\n' "$result" | tee -a "$output"
    sample=$((sample + 1))
done
