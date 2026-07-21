package main

import "testing"

func TestEvaluateAcceptsFasterPairedResults(t *testing.T) {
	values := samples{}
	for _, family := range []string{"BenchmarkParse", "BenchmarkParseHTML"} {
		for _, fixture := range []string{"small", "korean"} {
			base := family + "/" + fixture
			values[base+"/markonward"] = []metrics{{time: 80, bytes: 70, allocations: 5}}
			values[base+"/goldmark"] = []metrics{{time: 100, bytes: 100, allocations: 10}}
		}
	}
	if err := evaluate(values, 1); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluateRejectsOneFixtureRegression(t *testing.T) {
	values := samples{
		"BenchmarkParse/small/markonward": {{time: 116, bytes: 50, allocations: 5}},
		"BenchmarkParse/small/goldmark":   {{time: 100, bytes: 100, allocations: 10}},
	}
	if err := evaluate(values, 1); err == nil {
		t.Fatal("expected per-fixture regression to fail")
	}
}
