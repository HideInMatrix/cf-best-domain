package app

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type ProbeResult struct {
	IP            string    `json:"ip"`
	TCPLatencyMS  float64   `json:"tcp_latency_ms"`
	HTTPLatencyMS float64   `json:"http_latency_ms"`
	StatusCode    int       `json:"status_code"`
	OK            bool      `json:"ok"`
	ScoreMS       float64   `json:"score_ms"`
	CheckedAt     time.Time `json:"checked_at"`
	Error         string    `json:"error,omitempty"`
}

type Prober struct {
	Host      string
	Path      string
	Port      string
	Timeout   time.Duration
	UserAgent string
	BodyLimit int64
}

func (p Prober) ProbeAll(ctx context.Context, ips []string, concurrency int) []ProbeResult {
	if concurrency <= 0 {
		concurrency = 1
	}
	if concurrency > len(ips) {
		concurrency = len(ips)
	}
	if concurrency == 0 {
		return nil
	}

	jobs := make(chan string)
	results := make(chan ProbeResult, len(ips))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ip := range jobs {
				results <- p.Probe(ctx, ip)
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, ip := range ips {
			select {
			case <-ctx.Done():
				return
			case jobs <- ip:
			}
		}
	}()

	wg.Wait()
	close(results)

	out := make([]ProbeResult, 0, len(ips))
	for result := range results {
		out = append(out, result)
	}
	SortResults(out)
	return out
}

func (p Prober) Probe(ctx context.Context, ip string) ProbeResult {
	result := ProbeResult{IP: ip, CheckedAt: time.Now().UTC()}
	address := net.JoinHostPort(ip, p.Port)

	dialer := &net.Dialer{Timeout: p.Timeout}
	tcpCtx, cancelTCP := context.WithTimeout(ctx, p.Timeout)
	startTCP := time.Now()
	conn, err := dialer.DialContext(tcpCtx, "tcp", address)
	result.TCPLatencyMS = durationMS(time.Since(startTCP))
	cancelTCP()
	if err != nil {
		result.Error = cleanError(err)
		return result
	}
	_ = conn.Close()

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, _ string) (net.Conn, error) {
			return dialer.DialContext(ctx, network, address)
		},
		TLSClientConfig: &tls.Config{
			ServerName: p.Host,
			MinVersion: tls.VersionTLS12,
		},
		ResponseHeaderTimeout: p.Timeout,
		DisableKeepAlives:     true,
		ForceAttemptHTTP2:     true,
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   p.Timeout,
	}

	reqURL := url.URL{Scheme: "https", Host: p.Host, Path: p.Path}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		result.Error = cleanError(err)
		return result
	}
	if p.UserAgent != "" {
		req.Header.Set("User-Agent", p.UserAgent)
	}
	req.Header.Set("Cache-Control", "no-cache")

	startHTTP := time.Now()
	resp, err := client.Do(req)
	result.HTTPLatencyMS = durationMS(time.Since(startHTTP))
	if err != nil {
		result.Error = cleanError(err)
		return result
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, p.BodyLimit))
	result.StatusCode = resp.StatusCode
	if resp.StatusCode >= 200 && resp.StatusCode < 500 {
		result.OK = true
		result.ScoreMS = result.TCPLatencyMS + result.HTTPLatencyMS
		return result
	}

	result.Error = fmt.Sprintf("HTTP 状态码异常: %d", resp.StatusCode)
	return result
}

func SortResults(results []ProbeResult) {
	sort.SliceStable(results, func(i, j int) bool {
		left, right := results[i], results[j]
		if left.OK != right.OK {
			return left.OK
		}
		if left.OK && right.OK && left.ScoreMS != right.ScoreMS {
			return left.ScoreMS < right.ScoreMS
		}
		if left.TCPLatencyMS != right.TCPLatencyMS {
			return left.TCPLatencyMS < right.TCPLatencyMS
		}
		return left.IP < right.IP
	})
}

func BestResult(results []ProbeResult) *ProbeResult {
	for i := range results {
		if results[i].OK {
			return &results[i]
		}
	}
	return nil
}

func LimitResults(results []ProbeResult, limit int) []ProbeResult {
	if limit <= 0 || limit >= len(results) {
		return append([]ProbeResult(nil), results...)
	}
	return append([]ProbeResult(nil), results[:limit]...)
}

func durationMS(value time.Duration) float64 {
	return float64(value.Microseconds()) / 1000
}

func cleanError(err error) string {
	msg := err.Error()
	if len(msg) > 240 {
		msg = msg[:240]
	}
	return strings.ReplaceAll(msg, "\n", " ")
}
