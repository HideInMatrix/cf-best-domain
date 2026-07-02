package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type scanManager struct {
	cfg Config
	log io.Writer

	mu             sync.RWMutex
	running        bool
	report         *ScanReport
	lastError      string
	lastStartedAt  time.Time
	lastFinishedAt time.Time
	hasStarted     bool
	hasFinished    bool
	wg             sync.WaitGroup
}

type apiSnapshot struct {
	Running        bool        `json:"running"`
	LastStartedAt  *time.Time  `json:"last_started_at,omitempty"`
	LastFinishedAt *time.Time  `json:"last_finished_at,omitempty"`
	LastError      string      `json:"last_error,omitempty"`
	Report         *ScanReport `json:"report,omitempty"`
}

type apiError struct {
	Error     string `json:"error"`
	Running   bool   `json:"running,omitempty"`
	LastError string `json:"last_error,omitempty"`
}

type bestResponse struct {
	IP            string      `json:"ip"`
	Best          ProbeResult `json:"best"`
	CandidateIPs  int         `json:"candidate_ips"`
	SuccessfulIPs int         `json:"successful_ips"`
	DNS           *DNSUpdate  `json:"dns,omitempty"`
	CheckedAt     time.Time   `json:"checked_at"`
}

type ipsResponse struct {
	IPs           []string      `json:"ips"`
	Results       []ProbeResult `json:"results"`
	CandidateIPs  int           `json:"candidate_ips"`
	SuccessfulIPs int           `json:"successful_ips"`
	CheckedAt     time.Time     `json:"checked_at"`
}

func Serve(ctx context.Context, cfg Config, w io.Writer) error {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return err
	}
	if w == nil {
		w = io.Discard
	}
	serverCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	scans := newScanManager(cfg, w)
	handler := newAPIHandler(serverCtx, scans, cfg.Top)

	server := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}

	scans.Start(serverCtx)
	go scans.schedule(serverCtx)

	errCh := make(chan error, 1)
	go func() {
		fmt.Fprintf(w, "HTTP API 已启动，监听地址：%s\n", cfg.Listen)
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		scans.Wait()
		return ctx.Err()
	case err := <-errCh:
		cancel()
		scans.Wait()
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func newScanManager(cfg Config, log io.Writer) *scanManager {
	if log == nil {
		log = io.Discard
	}
	return &scanManager{cfg: cfg.Normalized(), log: log}
}

func (m *scanManager) Start(ctx context.Context) bool {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return false
	}
	now := time.Now().UTC()
	m.running = true
	m.hasStarted = true
	m.lastStartedAt = now
	m.lastError = ""
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()

		fmt.Fprintln(m.log, "开始测速...")
		report, err := Scan(ctx, m.cfg)
		if !report.CheckedAt.IsZero() {
			if writeErr := WriteReport(m.log, m.cfg.Output, report, m.cfg.Top); writeErr != nil {
				if err == nil {
					err = writeErr
				} else {
					fmt.Fprintf(m.log, "输出测速报告失败：%v\n", writeErr)
				}
			}
		}
		if err != nil {
			fmt.Fprintf(m.log, "测速失败：%v\n", err)
		}

		m.mu.Lock()
		if !report.CheckedAt.IsZero() {
			m.report = cloneScanReport(&report)
		}
		if err != nil {
			m.lastError = err.Error()
		} else {
			m.lastError = ""
		}
		m.running = false
		m.hasFinished = true
		m.lastFinishedAt = time.Now().UTC()
		m.mu.Unlock()
	}()
	return true
}

func (m *scanManager) schedule(ctx context.Context) {
	if m.cfg.Interval <= 0 {
		return
	}

	ticker := time.NewTicker(m.cfg.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !m.Start(ctx) {
				fmt.Fprintln(m.log, "上一轮测速仍在进行，跳过本次定时扫描")
			}
		}
	}
}

func (m *scanManager) Snapshot() apiSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snapshot := apiSnapshot{
		Running:   m.running,
		LastError: m.lastError,
		Report:    cloneScanReport(m.report),
	}
	if m.hasStarted {
		snapshot.LastStartedAt = timePtr(m.lastStartedAt)
	}
	if m.hasFinished {
		snapshot.LastFinishedAt = timePtr(m.lastFinishedAt)
	}
	return snapshot
}

func (m *scanManager) Wait() {
	m.wg.Wait()
}

func newAPIHandler(ctx context.Context, scans *scanManager, defaultTop int) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet, http.MethodHead) {
			return
		}
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return
		}
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	mux.HandleFunc("/api/report", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		writeJSON(w, http.StatusOK, scans.Snapshot())
	})
	mux.HandleFunc("/api/best", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		snapshot := scans.Snapshot()
		report := snapshot.Report
		if report == nil || report.Best == nil {
			writeJSON(w, http.StatusServiceUnavailable, apiError{
				Error:     "还没有可用的优选 IP",
				Running:   snapshot.Running,
				LastError: snapshot.LastError,
			})
			return
		}
		writeJSON(w, http.StatusOK, bestResponse{
			IP:            report.Best.IP,
			Best:          *report.Best,
			CandidateIPs:  report.CandidateIPs,
			SuccessfulIPs: report.SuccessfulIPs,
			DNS:           report.DNS,
			CheckedAt:     report.CheckedAt,
		})
	})
	mux.HandleFunc("/api/ips", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodGet) {
			return
		}
		snapshot := scans.Snapshot()
		report := snapshot.Report
		if report == nil || report.Best == nil {
			writeJSON(w, http.StatusServiceUnavailable, apiError{
				Error:     "还没有可用的优选 IP",
				Running:   snapshot.Running,
				LastError: snapshot.LastError,
			})
			return
		}

		results := successfulResults(report.Results)
		results = LimitResults(results, requestTop(r, defaultTop))
		ips := make([]string, 0, len(results))
		for _, result := range results {
			ips = append(ips, result.IP)
		}
		writeJSON(w, http.StatusOK, ipsResponse{
			IPs:           ips,
			Results:       results,
			CandidateIPs:  report.CandidateIPs,
			SuccessfulIPs: report.SuccessfulIPs,
			CheckedAt:     report.CheckedAt,
		})
	})
	mux.HandleFunc("/api/scan", func(w http.ResponseWriter, r *http.Request) {
		if !allowMethod(w, r, http.MethodPost) {
			return
		}
		if !scans.Start(ctx) {
			writeJSON(w, http.StatusConflict, apiError{Error: "测速正在进行中", Running: true})
			return
		}
		writeJSON(w, http.StatusAccepted, scans.Snapshot())
	})
	return mux
}

func allowMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	w.Header().Set("Allow", strings.Join(methods, ", "))
	writeJSON(w, http.StatusMethodNotAllowed, apiError{Error: "不支持的请求方法"})
	return false
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func requestTop(r *http.Request, fallback int) int {
	if fallback <= 0 {
		fallback = 10
	}
	value := strings.TrimSpace(r.URL.Query().Get("top"))
	if value == "" {
		return fallback
	}
	top, err := strconv.Atoi(value)
	if err != nil || top <= 0 {
		return fallback
	}
	return top
}

func successfulResults(results []ProbeResult) []ProbeResult {
	out := make([]ProbeResult, 0, len(results))
	for _, result := range results {
		if result.OK {
			out = append(out, result)
		}
	}
	return out
}

func cloneScanReport(report *ScanReport) *ScanReport {
	if report == nil {
		return nil
	}
	clone := *report
	clone.Results = append([]ProbeResult(nil), report.Results...)
	if report.Best != nil {
		best := *report.Best
		clone.Best = &best
	}
	if report.DNS != nil {
		dns := *report.DNS
		clone.DNS = &dns
	}
	return &clone
}

func timePtr(value time.Time) *time.Time {
	return &value
}
