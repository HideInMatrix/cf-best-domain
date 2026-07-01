package app

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"
	"time"
)

type ScanReport struct {
	Version       string        `json:"version"`
	CandidateIPs  int           `json:"candidate_ips"`
	SuccessfulIPs int           `json:"successful_ips"`
	Best          *ProbeResult  `json:"best,omitempty"`
	Results       []ProbeResult `json:"results"`
	DNS           *DNSUpdate    `json:"dns,omitempty"`
	CheckedAt     time.Time     `json:"checked_at"`
}

func WriteReport(w io.Writer, format string, report ScanReport, top int) error {
	report.Results = LimitResults(report.Results, top)
	switch format {
	case "json":
		encoder := json.NewEncoder(w)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	case "table":
		return writeTableReport(w, report)
	default:
		return fmt.Errorf("不支持的输出格式 %q", format)
	}
}

func writeTableReport(w io.Writer, report ScanReport) error {
	fmt.Fprintf(w, "候选 IP 数量: %d, 可用 IP 数量: %d\n", report.CandidateIPs, report.SuccessfulIPs)
	if report.Best != nil {
		fmt.Fprintf(w, "最快 IP: %s（总评分 %.1f ms）\n\n", report.Best.IP, report.Best.ScoreMS)
	} else {
		fmt.Fprintln(w, "最快 IP: 无")
		fmt.Fprintln(w)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "序号\tIP 地址\tTCP 延迟\tHTTPS 延迟\t状态码\t总评分\t错误信息")
	for i, result := range report.Results {
		errText := result.Error
		if errText == "" {
			errText = "-"
		}
		fmt.Fprintf(
			tw,
			"%d\t%s\t%.1fms\t%.1fms\t%d\t%.1fms\t%s\n",
			i+1,
			result.IP,
			result.TCPLatencyMS,
			result.HTTPLatencyMS,
			result.StatusCode,
			result.ScoreMS,
			errText,
		)
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	if report.DNS != nil {
		fmt.Fprintf(
			w,
			"\nDNS %s：%s -> %s，代理=%s，TTL=%d\n",
			dnsActionText(report.DNS.Action),
			report.DNS.RecordName,
			report.DNS.NewContent,
			proxyText(report.DNS.Proxied),
			report.DNS.TTL,
		)
	}
	return nil
}

func dnsActionText(action string) string {
	switch action {
	case "created":
		return "已创建"
	case "updated":
		return "已更新"
	case "unchanged":
		return "无需更新"
	default:
		return action
	}
}

func proxyText(proxied bool) string {
	if proxied {
		return "开启"
	}
	return "关闭（DNS-only）"
}
