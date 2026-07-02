package app

import (
	"context"
	"fmt"
	"io"
	"time"
)

func Run(ctx context.Context, cfg Config, w io.Writer) error {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return err
	}

	if cfg.Interval == 0 {
		return RunOnce(ctx, cfg, w)
	}

	for {
		if err := RunOnce(ctx, cfg, w); err != nil {
			fmt.Fprintf(w, "测速失败：%v\n", err)
		}
		fmt.Fprintf(w, "等待 %s 后开始下一轮测速...\n", cfg.Interval)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cfg.Interval):
		}
	}
}

func RunOnce(ctx context.Context, cfg Config, w io.Writer) error {
	cfg = cfg.Normalized()
	report, err := Scan(ctx, cfg)
	if !report.CheckedAt.IsZero() {
		if writeErr := WriteReport(w, cfg.Output, report, cfg.Top); writeErr != nil {
			return writeErr
		}
	}
	return err
}

func Scan(ctx context.Context, cfg Config) (ScanReport, error) {
	cfg = cfg.Normalized()
	if err := cfg.Validate(); err != nil {
		return ScanReport{}, err
	}

	cidrs, err := LoadIPv4CIDRs(ctx, cfg)
	if err != nil {
		return ScanReport{}, err
	}

	ips := SampleIPs(cidrs, cfg.SampleEach, cfg.MaxCandidates)
	if len(ips) == 0 {
		return ScanReport{}, fmt.Errorf("没有候选 IP")
	}

	prober := Prober{
		Host:      cfg.TestHost,
		Path:      cfg.TestPath,
		Port:      cfg.TestPort,
		Timeout:   cfg.Timeout,
		UserAgent: cfg.UserAgent,
		BodyLimit: cfg.BodyLimit,
	}
	results := prober.ProbeAll(ctx, ips, cfg.Concurrency)
	best := BestResult(results)
	successful := 0
	for _, result := range results {
		if result.OK {
			successful++
		}
	}

	report := ScanReport{
		Version:       cfg.Version,
		CandidateIPs:  len(ips),
		SuccessfulIPs: successful,
		Best:          best,
		Results:       results,
		CheckedAt:     time.Now().UTC(),
	}

	if best == nil {
		return report, fmt.Errorf("没有找到可用的 Cloudflare IP")
	}

	if cfg.UpdateDNS {
		dnsClient := NewCloudflareDNS(cfg.APIBase, cfg.APIToken)
		update, err := dnsClient.UpsertARecord(ctx, cfg.ZoneID, cfg.RecordName, best.IP, cfg.TTL, cfg.Proxied, cfg.Comment, cfg.CreateDNS)
		if err != nil {
			return report, err
		}
		report.DNS = &update
	}

	return report, nil
}
