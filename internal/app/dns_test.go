package app

import "testing"

func TestBuildARecordPayload(t *testing.T) {
	got := buildARecordPayload("cf-best.example.com", "192.0.2.10", 60, false, "managed")
	if got["type"] != "A" {
		t.Fatalf("记录类型 = %v，期望 A", got["type"])
	}
	if got["name"] != "cf-best.example.com" {
		t.Fatalf("记录名称 = %v", got["name"])
	}
	if got["content"] != "192.0.2.10" {
		t.Fatalf("记录内容 = %v", got["content"])
	}
	if got["proxied"] != false {
		t.Fatalf("代理状态 = %v", got["proxied"])
	}
	if got["ttl"] != 60 {
		t.Fatalf("TTL = %v", got["ttl"])
	}
	if got["comment"] != "managed" {
		t.Fatalf("备注 = %v", got["comment"])
	}
}
