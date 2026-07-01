package app

import "testing"

func TestSortResultsPrefersSuccessfulFastIP(t *testing.T) {
	results := []ProbeResult{
		{IP: "192.0.2.30", OK: true, ScoreMS: 80, TCPLatencyMS: 30},
		{IP: "192.0.2.20", OK: false, TCPLatencyMS: 10},
		{IP: "192.0.2.10", OK: true, ScoreMS: 20, TCPLatencyMS: 10},
	}

	SortResults(results)

	if results[0].IP != "192.0.2.10" {
		t.Fatalf("最快 IP = %q，期望 192.0.2.10", results[0].IP)
	}
	if results[2].IP != "192.0.2.20" {
		t.Fatalf("最后一个 IP = %q，期望 192.0.2.20", results[2].IP)
	}
}
