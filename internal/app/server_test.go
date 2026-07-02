package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAPIBestNotReady(t *testing.T) {
	handler := newAPIHandler(context.Background(), newScanManager(Config{}, io.Discard), 10)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/best", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("状态码 = %d，期望 %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestAPIBestReturnsLatestIP(t *testing.T) {
	handler := newAPIHandler(context.Background(), managerWithReport(), 10)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/best", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 %d，响应=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got bestResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.IP != "192.0.2.10" {
		t.Fatalf("优选 IP = %q，期望 192.0.2.10", got.IP)
	}
	if got.SuccessfulIPs != 2 {
		t.Fatalf("可用 IP 数量 = %d，期望 2", got.SuccessfulIPs)
	}
}

func TestAPIIPsHonorsTop(t *testing.T) {
	handler := newAPIHandler(context.Background(), managerWithReport(), 10)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ips?top=1", nil)

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d，期望 %d，响应=%s", rec.Code, http.StatusOK, rec.Body.String())
	}

	var got ipsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.IPs) != 1 || got.IPs[0] != "192.0.2.10" {
		t.Fatalf("IP 列表 = %#v，期望只包含 192.0.2.10", got.IPs)
	}
	if len(got.Results) != 1 {
		t.Fatalf("结果数量 = %d，期望 1", len(got.Results))
	}
}

func managerWithReport() *scanManager {
	results := []ProbeResult{
		{IP: "192.0.2.10", OK: true, ScoreMS: 12, TCPLatencyMS: 4, HTTPLatencyMS: 8},
		{IP: "192.0.2.20", OK: true, ScoreMS: 30, TCPLatencyMS: 10, HTTPLatencyMS: 20},
		{IP: "192.0.2.30", OK: false, Error: "timeout"},
	}
	report := ScanReport{
		Version:       "test",
		CandidateIPs:  len(results),
		SuccessfulIPs: 2,
		Results:       results,
		CheckedAt:     time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC),
	}
	report.Best = &report.Results[0]

	manager := newScanManager(Config{}, io.Discard)
	manager.report = &report
	return manager
}
