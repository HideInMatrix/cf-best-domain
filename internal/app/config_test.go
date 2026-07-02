package app

import "testing"

func TestNormalizeHostAcceptsURL(t *testing.T) {
	cfg := Config{TestHost: "https://www.example.com/cdn-cgi/trace", TestPath: "cdn-cgi/trace"}.Normalized()
	if cfg.TestHost != "www.example.com" {
		t.Fatalf("域名 = %q，期望 www.example.com", cfg.TestHost)
	}
	if cfg.TestPath != "/cdn-cgi/trace" {
		t.Fatalf("路径 = %q，期望 /cdn-cgi/trace", cfg.TestPath)
	}
}

func TestNormalizeUsesDefaultTestHost(t *testing.T) {
	cfg := Config{TestPath: "/cdn-cgi/trace"}.Normalized()
	if cfg.TestHost != DefaultTestHost {
		t.Fatalf("默认测速域名 = %q，期望 %q", cfg.TestHost, DefaultTestHost)
	}
}

func TestValidateRequiresDNSConfigWhenUpdating(t *testing.T) {
	cfg := Config{
		TestHost:      "www.example.com",
		TestPath:      "/",
		TestPort:      "443",
		SampleEach:    1,
		MaxCandidates: 1,
		Concurrency:   1,
		Timeout:       1,
		BodyLimit:     1,
		Output:        "table",
		Top:           1,
		UpdateDNS:     true,
		TTL:           60,
	}.Normalized()
	if err := cfg.Validate(); err == nil {
		t.Fatal("期望得到配置校验错误")
	}
}
