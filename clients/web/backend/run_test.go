package webconsole

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/Kantemba/clawy/pkg/config"
)

func TestRunHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, Run(ctx, Options{}), context.Canceled)
}

func TestRunServesEmbeddedOnboardingAndShutsDown(t *testing.T) {
	if _, err := frontendFS.ReadFile("dist/index.html"); err != nil {
		t.Skip("run scripts/build-web.sh to test the built embedded UI")
	}
	home := t.TempDir()
	t.Setenv(config.EnvHome, home)
	t.Setenv(config.EnvConfig, "")
	t.Setenv(config.EnvBinary, "")
	t.Setenv("CLAWY_LAUNCHER_HOST", "")

	probe, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := probe.Addr().(*net.TCPAddr).Port
	addr := probe.Addr().String()
	require.NoError(t, probe.Close())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- Run(ctx, Options{Port: strconv.Itoa(port), PortSet: true, Host: "127.0.0.1", HostSet: true, NoBrowser: true, Console: true})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(20 * time.Second):
			t.Error("web console did not stop after context cancellation")
		}
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			t.Errorf("web listener was not released: %v", err)
		} else {
			_ = listener.Close()
		}
	})

	base := "http://" + addr
	client := &http.Client{Timeout: time.Second}
	require.Eventually(t, func() bool {
		res, err := client.Get(base + "/api/auth/status")
		if err != nil {
			return false
		}
		defer res.Body.Close()
		var status struct{ Initialized bool }
		return res.StatusCode == http.StatusOK && json.NewDecoder(res.Body).Decode(&status) == nil && !status.Initialized
	}, 10*time.Second, 50*time.Millisecond)

	setup, err := client.Post(base+"/api/auth/setup", "application/json", bytes.NewBufferString(`{"password":"smoke-test-password","confirm":"smoke-test-password"}`))
	require.NoError(t, err)
	defer setup.Body.Close()
	require.Equal(t, http.StatusOK, setup.StatusCode)
	cookies := setup.Cookies()
	require.NotEmpty(t, cookies)

	for _, path := range []string{"/setup", "/api/models", "/api/gateway/status"} {
		req, err := http.NewRequest(http.MethodGet, base+path, nil)
		require.NoError(t, err)
		for _, cookie := range cookies {
			req.AddCookie(cookie)
		}
		res, err := client.Do(req)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, res.StatusCode, path)
		_ = res.Body.Close()
	}
	_, err = os.Stat(filepath.Join(home, "config.json"))
	require.NoError(t, err)
}
