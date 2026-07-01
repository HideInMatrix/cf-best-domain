package app

import (
	"math/rand"
	"net"
	"testing"
)

func TestSplitList(t *testing.T) {
	got := SplitList("1.1.1.0/24, 2.2.2.0/24\n# 注释\n3.3.3.0/24;4.4.4.0/24")
	want := []string{"1.1.1.0/24", "2.2.2.0/24", "3.3.3.0/24", "4.4.4.0/24"}
	if len(got) != len(want) {
		t.Fatalf("解析到 %d 条，期望 %d 条：%#v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("第 %d 条 = %q，期望 %q", i, got[i], want[i])
		}
	}
}

func TestSampleIPsStaysInsideCIDR(t *testing.T) {
	cidrs, err := ParseIPv4CIDRs([]string{"192.0.2.0/29"})
	if err != nil {
		t.Fatal(err)
	}
	got := sampleIPsWithRand(cidrs, 20, 20, rand.New(rand.NewSource(1)))
	if len(got) == 0 {
		t.Fatal("期望抽样得到 IP")
	}
	for _, ip := range got {
		parsed := net.ParseIP(ip)
		if !cidrs[0].Contains(parsed) {
			t.Fatalf("%s 不在 %s 范围内", ip, cidrs[0])
		}
		if ip == "192.0.2.0" || ip == "192.0.2.7" {
			t.Fatalf("抽样到了网络地址或广播地址：%s", ip)
		}
	}
}
