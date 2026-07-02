package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"cf-best-domain/internal/app"
)

var (
	version = "开发版"
	commit  = "无"
	date    = "未知"
)

type stringList []string

func (s *stringList) String() string {
	return strings.Join(*s, ",")
}

func (s *stringList) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	cfg, showVersion, err := parseConfig(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if showVersion {
		fmt.Printf("cf-best-domain %s（提交 %s，构建时间 %s）\n", version, commit, date)
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var runErr error
	if cfg.API {
		runErr = app.Serve(ctx, cfg, os.Stdout)
	} else {
		runErr = app.Run(ctx, cfg, os.Stdout)
	}
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		fmt.Fprintf(os.Stderr, "程序运行失败：%v\n", runErr)
		os.Exit(1)
	}
}

func parseConfig(args []string) (app.Config, bool, error) {
	cfg := app.Config{
		Version:       version,
		Commit:        commit,
		BuildDate:     date,
		APIBase:       envString("CF_API_BASE", app.DefaultCloudflareAPIBase),
		APIToken:      envString("CF_API_TOKEN", ""),
		ZoneID:        envString("CF_ZONE_ID", ""),
		RecordName:    envString("CF_RECORD", ""),
		UpdateDNS:     envBoolAny([]string{"UPDATE", "CFBD_UPDATE"}, false),
		CreateDNS:     envBoolAny([]string{"CFBD_CREATE_DNS"}, true),
		Proxied:       envBoolAny([]string{"CF_PROXIED", "CFBD_PROXIED"}, false),
		TTL:           envIntAny([]string{"CF_TTL", "CFBD_TTL"}, 60),
		Comment:       envString("CF_COMMENT", "由 cf-best-domain 自动维护"),
		TestHost:      envString("TEST_HOST", app.DefaultTestHost),
		TestPath:      envString("TEST_PATH", "/cdn-cgi/trace"),
		TestPort:      envString("TEST_PORT", "443"),
		CIDRFile:      envString("CFBD_CIDR_FILE", ""),
		CIDRURL:       envString("CFBD_CIDR_URL", app.DefaultCloudflareIPv4URL),
		SampleEach:    envIntAny([]string{"SAMPLE", "CFBD_SAMPLE"}, 5),
		MaxCandidates: envIntAny([]string{"MAX", "CFBD_MAX"}, 200),
		Concurrency:   envIntAny([]string{"C", "CFBD_CONCURRENCY"}, 50),
		Timeout:       envDuration("CFBD_TIMEOUT", 3*time.Second),
		Interval:      envDuration("CFBD_INTERVAL", 0),
		BodyLimit:     int64(envIntAny([]string{"CFBD_BODY_LIMIT"}, 64*1024)),
		Output:        envString("CFBD_OUTPUT", "table"),
		Top:           envIntAny([]string{"TOP", "CFBD_TOP"}, 10),
		UserAgent:     envString("CFBD_USER_AGENT", "cf-best-domain/"+version),
		API:           envBoolAny([]string{"CFBD_API"}, false),
		Listen:        envString("CFBD_LISTEN", ":8080"),
	}

	var showVersion bool
	var cidrFlags stringList
	cidrList := envString("CFBD_CIDRS", "")

	fs := flag.NewFlagSet("cf-best-domain", flag.ContinueOnError)
	fs.Usage = func() { printUsage(fs, cfg) }
	fs.StringVar(&cfg.TestHost, "host", cfg.TestHost, "用于 HTTPS 测速的 Cloudflare 代理域名")
	fs.StringVar(&cfg.TestPath, "path", cfg.TestPath, "HTTPS 测速路径")
	fs.StringVar(&cfg.TestPort, "port", cfg.TestPort, "Cloudflare 边缘节点测速端口")
	fs.IntVar(&cfg.SampleEach, "sample", cfg.SampleEach, "每个 CIDR 随机抽样的 IP 数量")
	fs.IntVar(&cfg.MaxCandidates, "max", cfg.MaxCandidates, "最大候选 IP 数量")
	fs.IntVar(&cfg.Concurrency, "c", cfg.Concurrency, "并发测速数量")
	fs.Var(durationValue{target: &cfg.Timeout}, "timeout", "单个 IP 探测超时时间，支持 3s 或纯数字秒数")
	fs.StringVar(&cfg.Output, "output", cfg.Output, "输出格式：table 或 json")
	fs.IntVar(&cfg.Top, "top", cfg.Top, "显示前 N 条测速结果")
	fs.BoolVar(&cfg.API, "api", cfg.API, "启动 HTTP API 服务并常驻运行")
	fs.StringVar(&cfg.Listen, "listen", cfg.Listen, "HTTP API 监听地址")
	fs.BoolVar(&cfg.UpdateDNS, "update", cfg.UpdateDNS, "把最快 IP 更新到 Cloudflare DNS A 记录")
	fs.StringVar(&cfg.APIToken, "token", cfg.APIToken, "Cloudflare API Token，建议使用 CF_API_TOKEN 环境变量")
	fs.StringVar(&cfg.ZoneID, "zone", cfg.ZoneID, "Cloudflare Zone ID")
	fs.StringVar(&cfg.RecordName, "record", cfg.RecordName, "要更新的 DNS A 记录名称，例如 cf-best.example.com")
	fs.IntVar(&cfg.TTL, "ttl", cfg.TTL, "DNS TTL；1 表示自动，或设置为 60-86400 秒")
	fs.BoolVar(&cfg.Proxied, "proxied", cfg.Proxied, "是否开启 Cloudflare 代理；优选入口建议保持 DNS-only")
	fs.BoolVar(&cfg.CreateDNS, "create", cfg.CreateDNS, "记录不存在时自动创建")
	fs.StringVar(&cfg.Comment, "comment", cfg.Comment, "Cloudflare DNS 记录备注")
	fs.StringVar(&cfg.APIBase, "api-base", cfg.APIBase, "Cloudflare API 基础地址")
	fs.StringVar(&cfg.CIDRURL, "cidr-url", cfg.CIDRURL, "Cloudflare IPv4 段来源地址")
	fs.StringVar(&cfg.CIDRFile, "cidr-file", cfg.CIDRFile, "IPv4 CIDR 文件，每行一个")
	fs.StringVar(&cidrList, "cidrs", cidrList, "手动指定 IPv4 CIDR，支持逗号、分号或换行分隔")
	fs.Var(&cidrFlags, "cidr", "手动指定一个 IPv4 CIDR，可重复传入")
	fs.Var(durationValue{target: &cfg.Interval}, "interval", "定时测速间隔；0 表示只运行一次")
	fs.Int64Var(&cfg.BodyLimit, "body-limit", cfg.BodyLimit, "单次 HTTPS 响应最多读取的字节数")
	fs.StringVar(&cfg.UserAgent, "user-agent", cfg.UserAgent, "HTTPS 测速使用的 User-Agent")
	fs.BoolVar(&showVersion, "version", false, "显示版本号并退出")

	if err := fs.Parse(args); err != nil {
		return cfg, false, err
	}

	cfg.CIDRInputs = append(cfg.CIDRInputs, app.SplitList(cidrList)...)
	for _, value := range cidrFlags {
		cfg.CIDRInputs = append(cfg.CIDRInputs, app.SplitList(value)...)
	}

	return cfg, showVersion, nil
}

func printUsage(fs *flag.FlagSet, cfg app.Config) {
	fmt.Fprintf(fs.Output(), "cf-best-domain %s\n", version)
	fmt.Fprintln(fs.Output(), "扫描 Cloudflare IPv4 段，测试 TCP/HTTPS 延迟，选出最快 IP，并可更新 DNS-only A 记录。")
	fmt.Fprintln(fs.Output())
	fmt.Fprintln(fs.Output(), "用法：")
	fmt.Fprintln(fs.Output(), "  cf-best-domain")
	fmt.Fprintln(fs.Output(), "  cf-best-domain -host www.example.com -update")
	fmt.Fprintln(fs.Output(), "  cf-best-domain -api -update -interval 30m")
	fmt.Fprintln(fs.Output())
	fmt.Fprintln(fs.Output(), "参数：")
	fmt.Fprintf(fs.Output(), "  -host 文本          用于 HTTPS 测速的 Cloudflare 代理域名；不传则使用通用测速域名（默认：%q）\n", cfg.TestHost)
	fmt.Fprintf(fs.Output(), "  -path 文本          HTTPS 测速路径（默认：%q）\n", cfg.TestPath)
	fmt.Fprintf(fs.Output(), "  -port 文本          Cloudflare 边缘节点测速端口（默认：%q）\n", cfg.TestPort)
	fmt.Fprintf(fs.Output(), "  -sample 数字        每个 CIDR 随机抽样的 IP 数量（默认：%d）\n", cfg.SampleEach)
	fmt.Fprintf(fs.Output(), "  -max 数字           最大候选 IP 数量（默认：%d）\n", cfg.MaxCandidates)
	fmt.Fprintf(fs.Output(), "  -c 数字             并发测速数量（默认：%d）\n", cfg.Concurrency)
	fmt.Fprintf(fs.Output(), "  -timeout 时长       单个 IP 探测超时时间，支持 3s 或纯数字秒数（默认：%s）\n", cfg.Timeout)
	fmt.Fprintf(fs.Output(), "  -output 文本        输出格式：table 或 json（默认：%q）\n", cfg.Output)
	fmt.Fprintf(fs.Output(), "  -top 数字           显示前 N 条测速结果（默认：%d）\n", cfg.Top)
	fmt.Fprintf(fs.Output(), "  -api                启动 HTTP API 服务并常驻运行（默认：%s）\n", boolText(cfg.API))
	fmt.Fprintf(fs.Output(), "  -listen 文本        HTTP API 监听地址（默认：%q）\n", cfg.Listen)
	fmt.Fprintf(fs.Output(), "  -update             把最快 IP 更新到 Cloudflare DNS A 记录（默认：%s）\n", boolText(cfg.UpdateDNS))
	fmt.Fprintf(fs.Output(), "  -token 文本         Cloudflare API Token，建议使用 CF_API_TOKEN 环境变量\n")
	fmt.Fprintf(fs.Output(), "  -zone 文本          Cloudflare Zone ID（默认：%q）\n", cfg.ZoneID)
	fmt.Fprintf(fs.Output(), "  -record 文本        要更新的 DNS A 记录名称，例如 cf-best.example.com（默认：%q）\n", cfg.RecordName)
	fmt.Fprintf(fs.Output(), "  -ttl 数字           DNS TTL；1 表示自动，或设置为 60-86400 秒（默认：%d）\n", cfg.TTL)
	fmt.Fprintf(fs.Output(), "  -proxied            是否开启 Cloudflare 代理；优选入口建议保持 DNS-only（默认：%s）\n", boolText(cfg.Proxied))
	fmt.Fprintf(fs.Output(), "  -create             记录不存在时自动创建（默认：%s）\n", boolText(cfg.CreateDNS))
	fmt.Fprintf(fs.Output(), "  -comment 文本       Cloudflare DNS 记录备注（默认：%q）\n", cfg.Comment)
	fmt.Fprintf(fs.Output(), "  -api-base 文本      Cloudflare API 基础地址（默认：%q）\n", cfg.APIBase)
	fmt.Fprintf(fs.Output(), "  -cidr-url 文本      Cloudflare IPv4 段来源地址（默认：%q）\n", cfg.CIDRURL)
	fmt.Fprintf(fs.Output(), "  -cidr-file 文本     IPv4 CIDR 文件，每行一个（默认：%q）\n", cfg.CIDRFile)
	fmt.Fprintln(fs.Output(), "  -cidrs 文本         手动指定 IPv4 CIDR，支持逗号、分号或换行分隔")
	fmt.Fprintln(fs.Output(), "  -cidr 文本          手动指定一个 IPv4 CIDR，可重复传入")
	fmt.Fprintf(fs.Output(), "  -interval 时长      定时测速间隔；0 表示只运行一次（默认：%s）\n", cfg.Interval)
	fmt.Fprintf(fs.Output(), "  -body-limit 数字    单次 HTTPS 响应最多读取的字节数（默认：%d）\n", cfg.BodyLimit)
	fmt.Fprintf(fs.Output(), "  -user-agent 文本    HTTPS 测速使用的 User-Agent（默认：%q）\n", cfg.UserAgent)
	fmt.Fprintln(fs.Output(), "  -version            显示版本号并退出")
}

func boolText(value bool) string {
	if value {
		return "开启"
	}
	return "关闭"
}

func envString(key, fallback string) string {
	if value := cleanEnvValue(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := cleanEnvValue(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envIntAny(keys []string, fallback int) int {
	for _, key := range keys {
		value := cleanEnvValue(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, err := strconv.Atoi(value)
		if err == nil {
			return parsed
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := cleanEnvValue(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, ok := parseBool(value)
	if !ok {
		return fallback
	}
	return parsed
}

func envBoolAny(keys []string, fallback bool) bool {
	for _, key := range keys {
		value := cleanEnvValue(os.Getenv(key))
		if value == "" {
			continue
		}
		parsed, ok := parseBool(value)
		if ok {
			return parsed
		}
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := cleanEnvValue(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := parseFlexibleDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

type durationValue struct {
	target *time.Duration
}

func (d durationValue) String() string {
	if d.target == nil {
		return ""
	}
	return d.target.String()
}

func (d durationValue) Set(value string) error {
	parsed, err := parseFlexibleDuration(value)
	if err != nil {
		return err
	}
	*d.target = parsed
	return nil
}

func parseFlexibleDuration(value string) (time.Duration, error) {
	value = cleanEnvValue(value)
	if value == "" {
		return 0, fmt.Errorf("时长不能为空")
	}
	if seconds, err := strconv.Atoi(value); err == nil {
		return time.Duration(seconds) * time.Second, nil
	}
	return time.ParseDuration(value)
}

func cleanEnvValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			value = value[1 : len(value)-1]
		}
	}
	return strings.TrimSpace(value)
}

func parseBool(value string) (bool, bool) {
	switch strings.ToLower(cleanEnvValue(value)) {
	case "1", "t", "true", "y", "yes", "on", "enable", "enabled", "是", "真", "开启", "启用":
		return true, true
	case "0", "f", "false", "n", "no", "off", "disable", "disabled", "否", "假", "关闭", "禁用":
		return false, true
	default:
		return false, false
	}
}
