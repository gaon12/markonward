// Command benchgate enforces Markonward's comparative release thresholds from
// Go benchmark output containing paired markonward and goldmark sub-benchmarks.
package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

const maximumFixtureRegression = 1.15

var cpuSuffix = regexp.MustCompile(`-\d+$`)

type metrics struct {
	time        float64
	bytes       float64
	allocations float64
}

type samples map[string][]metrics

func main() {
	input := flag.String("input", "", "go test -bench output")
	minimum := flag.Int("samples", 10, "minimum samples required for every implementation")
	flag.Parse()
	if *input == "" {
		fatal(fmt.Errorf("-input is required"))
	}
	file, err := os.Open(*input) // #nosec G304 -- the operator selects the benchmark result file.
	if err != nil {
		fatal(err)
	}
	payload, readErr := io.ReadAll(file)
	closeErr := file.Close()
	if readErr != nil {
		fatal(readErr)
	}
	if closeErr != nil {
		fatal(closeErr)
	}
	parsed, parseErr := parse(strings.NewReader(decodeText(payload)))
	if parseErr != nil {
		fatal(parseErr)
	}
	if err := evaluate(parsed, *minimum); err != nil {
		fatal(err)
	}
}

func parse(reader io.Reader) (samples, error) {
	result := make(samples)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 || !strings.HasPrefix(fields[0], "Benchmark") {
			continue
		}
		name := cpuSuffix.ReplaceAllString(fields[0], "")
		parts := strings.Split(name, "/")
		if len(parts) != 3 || parts[2] != "markonward" && parts[2] != "goldmark" {
			continue
		}
		current := metrics{}
		for index := 2; index+1 < len(fields); index += 2 {
			value, err := strconv.ParseFloat(fields[index], 64)
			if err != nil {
				continue
			}
			switch fields[index+1] {
			case "ns/op":
				current.time = value
			case "B/op":
				current.bytes = value
			case "allocs/op":
				current.allocations = value
			}
		}
		if current.time <= 0 || current.bytes <= 0 || current.allocations <= 0 {
			return nil, fmt.Errorf("benchmark %s lacks ns/op, B/op, or allocs/op", name)
		}
		result[name] = append(result[name], current)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func decodeText(payload []byte) string {
	if len(payload) < 2 {
		return string(payload)
	}
	var order binary.ByteOrder
	switch {
	case bytes.HasPrefix(payload, []byte{0xff, 0xfe}):
		order = binary.LittleEndian
	case bytes.HasPrefix(payload, []byte{0xfe, 0xff}):
		order = binary.BigEndian
	default:
		return string(payload)
	}
	units := make([]uint16, 0, (len(payload)-2)/2)
	for index := 2; index+1 < len(payload); index += 2 {
		units = append(units, order.Uint16(payload[index:index+2]))
	}
	return string(utf16.Decode(units))
}

func evaluate(results samples, minimum int) error {
	if minimum <= 0 {
		return fmt.Errorf("sample minimum must be positive")
	}
	type familyRatios struct {
		time, bytes, allocations []float64
	}
	families := make(map[string]*familyRatios)
	var keys []string
	for key := range results {
		if strings.HasSuffix(key, "/markonward") {
			keys = append(keys, strings.TrimSuffix(key, "/markonward"))
		}
	}
	sort.Strings(keys)
	if len(keys) == 0 {
		return fmt.Errorf("no paired Markonward benchmarks found")
	}
	var failures []string
	for _, base := range keys {
		mark := results[base+"/markonward"]
		gold := results[base+"/goldmark"]
		if len(mark) < minimum || len(gold) < minimum {
			failures = append(failures, fmt.Sprintf("%s has %d/%d samples, want at least %d each", base, len(mark), len(gold), minimum))
			continue
		}
		markMean, goldMean := geometricMetrics(mark), geometricMetrics(gold)
		ratios := metrics{
			time: markMean.time / goldMean.time, bytes: markMean.bytes / goldMean.bytes,
			allocations: markMean.allocations / goldMean.allocations,
		}
		fmt.Printf("%-32s ns/op %.3fx  B/op %.3fx  allocs/op %.3fx\n", base, ratios.time, ratios.bytes, ratios.allocations)
		if ratios.time > maximumFixtureRegression || ratios.bytes > maximumFixtureRegression || ratios.allocations > maximumFixtureRegression {
			failures = append(failures, fmt.Sprintf("%s exceeds the 1.15x per-fixture ceiling", base))
		}
		family := strings.Split(base, "/")[0]
		aggregate := families[family]
		if aggregate == nil {
			aggregate = &familyRatios{}
			families[family] = aggregate
		}
		aggregate.time = append(aggregate.time, ratios.time)
		aggregate.bytes = append(aggregate.bytes, ratios.bytes)
		aggregate.allocations = append(aggregate.allocations, ratios.allocations)
	}
	familyNames := make([]string, 0, len(families))
	for family := range families {
		familyNames = append(familyNames, family)
	}
	sort.Strings(familyNames)
	for _, family := range familyNames {
		ratios := families[family]
		timeRatio := geometricMean(ratios.time)
		byteRatio := geometricMean(ratios.bytes)
		allocationRatio := geometricMean(ratios.allocations)
		fmt.Printf("%-32s ns/op %.3fx  B/op %.3fx  allocs/op %.3fx (geomean)\n", family, timeRatio, byteRatio, allocationRatio)
		if timeRatio >= 1 || byteRatio >= 1 || allocationRatio >= 1 {
			failures = append(failures, fmt.Sprintf("%s geometric mean is not lower for every metric", family))
		}
	}
	if len(failures) != 0 {
		return fmt.Errorf("benchmark release gate failed:\n- %s", strings.Join(failures, "\n- "))
	}
	return nil
}

func geometricMetrics(values []metrics) metrics {
	times := make([]float64, len(values))
	bytesValues := make([]float64, len(values))
	allocations := make([]float64, len(values))
	for index, value := range values {
		times[index] = value.time
		bytesValues[index] = value.bytes
		allocations[index] = value.allocations
	}
	return metrics{time: geometricMean(times), bytes: geometricMean(bytesValues), allocations: geometricMean(allocations)}
}

func geometricMean(values []float64) float64 {
	if len(values) == 0 {
		return math.NaN()
	}
	total := 0.0
	for _, value := range values {
		total += math.Log(value)
	}
	return math.Exp(total / float64(len(values)))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "benchgate:", err)
	os.Exit(1)
}
