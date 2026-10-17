package speaker

import (
	"net"
	"testing"
)

func ip(s string) net.IP { return net.ParseIP(s).To4() }

// scoreAddr 必须把 VPN 和容器网段打到 0，把家用 192.168 排到最前。
func TestScoreAddr(t *testing.T) {
	cases := []struct {
		name  string
		addr  string
		flags net.Flags
		want  int
	}{
		{"家用 wifi 192.168", "192.168.31.204", net.FlagBroadcast, 120},
		{"Mihomo VPN 198.18.0.1", "198.18.0.1", net.FlagPointToPoint, 0},
		{"Tailscale 100.64.x", "100.64.5.7", net.FlagPointToPoint, 0},
		{"docker 桥 172.17.0.1", "172.17.0.1", net.FlagBroadcast, 25},
		{"docker 桥 172.18.0.1", "172.18.0.1", net.FlagBroadcast, 25},
		{"docker 桥 172.20.0.1", "172.20.0.1", net.FlagBroadcast, 25},
		{"172.25 非 docker 段", "172.25.0.1", net.FlagBroadcast, 60},
		{"10 段企业网", "10.0.0.5", net.FlagBroadcast, 60},
		{"10 段无广播", "10.0.0.5", 0, 40},
		{"link-local 169.254", "169.254.1.1", net.FlagBroadcast, 0},
		{"公网地址", "8.8.8.8", net.FlagBroadcast, 0},
		{"空 IP", "", 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := scoreAddr(ip(c.addr), c.flags); got != c.want {
				t.Errorf("scoreAddr(%s, %v) = %d, want %d", c.addr, c.flags, got, c.want)
			}
		})
	}
}

// lanIP 必须在本机返回非 VPN 地址：不能是 Mihomo(198.18.x) 或 Tailscale(100.64.x)。
func TestLanIPReal(t *testing.T) {
	got := lanIP()
	if got == "" {
		t.Fatal("lanIP() 返回空：本机应该有可用局域网地址")
	}
	a := ip(got)
	if a == nil {
		t.Fatalf("lanIP() 返回非 IPv4: %q", got)
	}
	if a[0] == 198 && a[1]&0xFE == 18 {
		t.Errorf("lanIP() 返回 Mihomo/CGNAT 段: %s", got)
	}
	if a[0] == 100 && a[1]&0xC0 == 64 {
		t.Errorf("lanIP() 返回 Tailscale 段: %s", got)
	}
	if a[0] == 169 && a[1] == 254 {
		t.Errorf("lanIP() 返回 link-local: %s", got)
	}
	t.Logf("lanIP() = %s", got)
}

// 关键回归：Mihomo 存在时仍必须选到 192.168，而不是 198.18。
func TestLanIPPrefersHomeOverVPN(t *testing.T) {
	home := scoreAddr(ip("192.168.31.204"), net.FlagBroadcast)
	vpn := scoreAddr(ip("198.18.0.1"), net.FlagPointToPoint)
	if !(home > vpn) {
		t.Errorf("期望家用(%d) > VPN(%d)，实际不成立", home, vpn)
	}
	if vpn != 0 {
		t.Errorf("VPN 地址必须被排除（score=0），实际 %d", vpn)
	}
}
