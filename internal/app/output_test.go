package app

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestWriteReportJSON(t *testing.T) {
	var buf bytes.Buffer
	report := ScanReport{
		Version:      "test",
		CandidateIPs: 1,
		Results: []ProbeResult{
			{IP: "192.0.2.1", OK: true, ScoreMS: 12.3},
		},
	}
	report.Best = &report.Results[0]

	if err := WriteReport(&buf, "json", report, 10); err != nil {
		t.Fatal(err)
	}
	if !json.Valid(buf.Bytes()) {
		t.Fatalf("JSON 无效：%s", buf.String())
	}
}
