package app

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"time"
)

func LoadIPv4CIDRs(ctx context.Context, cfg Config) ([]*net.IPNet, error) {
	inputs, err := cfg.LoadCIDRInputs()
	if err != nil {
		return nil, err
	}
	if len(inputs) == 0 {
		inputs, err = FetchCloudflareIPv4CIDRs(ctx, cfg.CIDRURL)
		if err != nil {
			return nil, err
		}
	}
	return ParseIPv4CIDRs(inputs)
}

func FetchCloudflareIPv4CIDRs(ctx context.Context, sourceURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("拉取 Cloudflare IPv4 段失败: %s", resp.Status)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return SplitList(string(body)), nil
}

func ParseIPv4CIDRs(inputs []string) ([]*net.IPNet, error) {
	var cidrs []*net.IPNet
	for _, input := range inputs {
		value := strings.TrimSpace(input)
		if value == "" {
			continue
		}
		ip, ipnet, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("解析 CIDR %q 失败: %w", value, err)
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("CIDR %q 不是 IPv4 段", value)
		}
		ipnet.IP = ipnet.IP.To4()
		cidrs = append(cidrs, ipnet)
	}
	if len(cidrs) == 0 {
		return nil, fmt.Errorf("没有可用的 IPv4 CIDR")
	}
	return cidrs, nil
}

func SampleIPs(cidrs []*net.IPNet, sampleEach int, maxTotal int) []string {
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return sampleIPsWithRand(cidrs, sampleEach, maxTotal, rng)
}

func sampleIPsWithRand(cidrs []*net.IPNet, sampleEach int, maxTotal int, rng *rand.Rand) []string {
	if sampleEach <= 0 || maxTotal <= 0 {
		return nil
	}

	var ips []string
	for _, ipnet := range cidrs {
		ones, bits := ipnet.Mask.Size()
		if bits != 32 {
			continue
		}
		hostBits := 32 - ones
		if hostBits <= 1 {
			continue
		}

		total := uint64(1) << uint(hostBits)
		base := binary.BigEndian.Uint32(ipnet.IP.To4())
		for i := 0; i < sampleEach; i++ {
			if len(ips) >= maxTotal {
				return uniqueStrings(ips)
			}
			offset := uint32(1 + rng.Int63n(int64(total-2)))
			var raw [4]byte
			binary.BigEndian.PutUint32(raw[:], base+offset)
			candidate := net.IPv4(raw[0], raw[1], raw[2], raw[3])
			if ipnet.Contains(candidate) {
				ips = append(ips, candidate.String())
			}
		}
	}
	return uniqueStrings(ips)
}

func SplitList(value string) []string {
	var out []string
	for _, line := range strings.Split(value, "\n") {
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		for _, field := range strings.FieldsFunc(line, func(r rune) bool {
			return r == ',' || r == ';' || r == '\t' || r == ' '
		}) {
			field = strings.TrimSpace(field)
			if field != "" {
				out = append(out, field)
			}
		}
	}
	return out
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}
