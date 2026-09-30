package main

import (
	"context"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeResolverPreservesLocalhost(t *testing.T) {
	if runtime.GOOS != "windows" && runtime.GOOS != "darwin" {
		t.Skip("native desktop resolver regression")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, "localhost")
	require.NoError(t, err, "embedded chat proxy must resolve localhost without querying public DNS")
	require.NotEmpty(t, addresses)
	for _, address := range addresses {
		assert.True(t, address.IP.IsLoopback(), "localhost resolved to %s", address.IP)
	}
}

func TestShouldUseFallbackDNS(t *testing.T) {
	for _, goos := range []string{"windows", "darwin", "freebsd", "netbsd", "linux", "android"} {
		t.Run(goos, func(t *testing.T) {
			assert.False(t, shouldUseFallbackDNS(goos, nil), "keep the system resolver when resolv.conf exists")
			assert.False(t, shouldUseFallbackDNS(goos, os.ErrPermission), "a permission error is not a missing DNS configuration")
			want := goos == "linux" || goos == "android"
			assert.Equal(t, want, shouldUseFallbackDNS(goos, os.ErrNotExist))
		})
	}
}
