package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// shouldUseFallbackDNS limits the no-resolv.conf workaround to platforms that
// actually use that file. Windows and macOS must keep their native resolver,
// including local hosts-file resolution for the embedded web console.
func shouldUseFallbackDNS(goos string, resolvErr error) bool {
	return (goos == "android" || goos == "linux") && os.IsNotExist(resolvErr)
}

func init() {
	_, err := os.Stat("/etc/resolv.conf")
	if !shouldUseFallbackDNS(runtime.GOOS, err) {
		return
	}

	// 从环境变量获取 DNS server 列表，多个用 ; 隔开
	// 例如: CLAWY_DNS_SERVER="8.8.8.8:53;1.1.1.1:53;223.5.5.5:53"
	dnsEnv := os.Getenv("CLAWY_DNS_SERVER")
	if dnsEnv == "" {
		dnsEnv = "8.8.8.8:53;1.1.1.1:53"
	}

	var dnsServers []string
	for _, s := range strings.Split(dnsEnv, ";") {
		s = strings.TrimSpace(s)
		if s != "" {
			// 如果没有带端口号，自动补上 :53
			if _, _, err := net.SplitHostPort(s); err != nil {
				s = s + ":53"
			}
			dnsServers = append(dnsServers, s)
		}
	}

	// A whitespace-only override must not leave an empty server list.
	if len(dnsServers) == 0 {
		dnsServers = []string{"8.8.8.8:53", "1.1.1.1:53"}
	}

	// 轮询索引，在多个 DNS 服务器之间轮转
	var idx uint64

	customResolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			// Round-robin: 依次尝试不同的 DNS 服务器
			server := dnsServers[atomic.AddUint64(&idx, 1)%uint64(len(dnsServers))]
			return d.DialContext(ctx, "udp", server)
		},
	}

	// 覆盖全局 DefaultResolver
	net.DefaultResolver = customResolver

	// 覆盖 http.DefaultTransport 使用自定义 DNS 解析的 DialContext
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		Resolver:  customResolver,
	}

	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		tr.DialContext = dialer.DialContext
	}
}
