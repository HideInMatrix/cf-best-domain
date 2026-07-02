package app

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultCloudflareAPIBase = "https://api.cloudflare.com/client/v4"
	DefaultCloudflareIPv4URL = "https://www.cloudflare.com/ips-v4"
	DefaultTestHost          = "speed.cloudflare.com"
)

type Config struct {
	Version   string
	Commit    string
	BuildDate string

	APIBase    string
	APIToken   string
	ZoneID     string
	RecordName string
	UpdateDNS  bool
	CreateDNS  bool
	Proxied    bool
	TTL        int
	Comment    string

	TestHost string
	TestPath string
	TestPort string

	CIDRInputs []string
	CIDRFile   string
	CIDRURL    string

	SampleEach    int
	MaxCandidates int
	Concurrency   int
	Timeout       time.Duration
	Interval      time.Duration
	BodyLimit     int64

	Output    string
	Top       int
	UserAgent string
}

func (c Config) Normalized() Config {
	c.APIBase = strings.TrimRight(strings.TrimSpace(c.APIBase), "/")
	c.APIToken = strings.TrimSpace(c.APIToken)
	c.ZoneID = strings.TrimSpace(c.ZoneID)
	c.RecordName = strings.TrimSpace(c.RecordName)
	c.TestHost = normalizeHost(c.TestHost)
	c.TestPath = strings.TrimSpace(c.TestPath)
	c.TestPort = strings.TrimSpace(c.TestPort)
	c.CIDRFile = strings.TrimSpace(c.CIDRFile)
	c.CIDRURL = strings.TrimSpace(c.CIDRURL)
	c.Output = strings.ToLower(strings.TrimSpace(c.Output))
	c.UserAgent = strings.TrimSpace(c.UserAgent)
	c.Comment = strings.TrimSpace(c.Comment)

	if c.APIBase == "" {
		c.APIBase = DefaultCloudflareAPIBase
	}
	if c.CIDRURL == "" {
		c.CIDRURL = DefaultCloudflareIPv4URL
	}
	if c.TestHost == "" {
		c.TestHost = DefaultTestHost
	}
	if c.TestPath == "" {
		c.TestPath = "/cdn-cgi/trace"
	}
	if !strings.HasPrefix(c.TestPath, "/") {
		c.TestPath = "/" + c.TestPath
	}
	if c.TestPort == "" {
		c.TestPort = "443"
	}
	if c.Output == "" {
		c.Output = "table"
	}
	if c.UserAgent == "" {
		c.UserAgent = "cf-best-domain/" + c.Version
	}
	if c.Comment == "" {
		c.Comment = "由 cf-best-domain 自动维护"
	}
	return c
}

func (c Config) Validate() error {
	var errs []error
	if _, err := strconv.Atoi(c.TestPort); err != nil {
		errs = append(errs, fmt.Errorf("测速端口 %q 无效", c.TestPort))
	}
	if c.SampleEach <= 0 {
		errs = append(errs, errors.New("sample 必须大于 0"))
	}
	if c.MaxCandidates <= 0 {
		errs = append(errs, errors.New("max 必须大于 0"))
	}
	if c.Concurrency <= 0 {
		errs = append(errs, errors.New("并发数量必须大于 0"))
	}
	if c.Timeout <= 0 {
		errs = append(errs, errors.New("timeout 必须大于 0"))
	}
	if c.Interval < 0 {
		errs = append(errs, errors.New("interval 不能为负数"))
	}
	if c.BodyLimit <= 0 {
		errs = append(errs, errors.New("body-limit 必须大于 0"))
	}
	if c.Top <= 0 {
		errs = append(errs, errors.New("top 必须大于 0"))
	}
	switch c.Output {
	case "table", "json":
	default:
		errs = append(errs, fmt.Errorf("不支持的输出格式 %q", c.Output))
	}
	if c.UpdateDNS {
		if c.APIToken == "" {
			errs = append(errs, errors.New("缺少 CF_API_TOKEN"))
		}
		if c.ZoneID == "" {
			errs = append(errs, errors.New("缺少 CF_ZONE_ID 环境变量或 -zone 参数"))
		}
		if c.RecordName == "" {
			errs = append(errs, errors.New("缺少 CF_RECORD 环境变量或 -record 参数"))
		}
		if c.TTL != 1 && (c.TTL < 60 || c.TTL > 86400) {
			errs = append(errs, errors.New("ttl 必须为 1（自动）或 60-86400 秒"))
		}
	}
	return errors.Join(errs...)
}

func (c Config) LoadCIDRInputs() ([]string, error) {
	inputs := append([]string(nil), c.CIDRInputs...)
	if c.CIDRFile != "" {
		data, err := os.ReadFile(c.CIDRFile)
		if err != nil {
			return nil, fmt.Errorf("读取 CIDR 文件失败: %w", err)
		}
		inputs = append(inputs, SplitList(string(data))...)
	}
	return inputs, nil
}

func normalizeHost(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.Contains(value, "://") {
		parsed, err := url.Parse(value)
		if err == nil && parsed.Host != "" {
			return parsed.Hostname()
		}
	}
	value = strings.TrimSuffix(value, ".")
	if host, _, ok := strings.Cut(value, "/"); ok {
		value = host
	}
	return value
}
