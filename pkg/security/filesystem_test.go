package security

import (
	"runtime"
	"testing"
)

func TestFilesystemSandbox(t *testing.T) {
	workspace := t.TempDir()
	sandbox := NewFilesystemSandbox(workspace)

	t.Run("allow read within workspace", func(t *testing.T) {
		err := sandbox.ValidateRead(workspace + "/test.txt")
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("block read outside workspace", func(t *testing.T) {
		err := sandbox.ValidateRead("/etc/passwd")
		if err == nil {
			t.Error("expected error for path outside workspace")
		}
	})

	t.Run("block system paths", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix-specific test")
		}
		err := sandbox.ValidateRead("/etc/shadow")
		if err == nil {
			t.Error("expected error for system path")
		}
	})

	t.Run("allow write within workspace", func(t *testing.T) {
		err := sandbox.ValidateWrite(workspace + "/output.txt")
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	})

	t.Run("block delete of system path", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("unix-specific test")
		}
		err := sandbox.ValidateDelete("/etc/passwd")
		if err == nil {
			t.Error("expected error for deleting system path")
		}
	})

	t.Run("empty path", func(t *testing.T) {
		err := sandbox.ValidateRead("")
		if err == nil {
			t.Error("expected error for empty path")
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		err := sandbox.ValidateRead(workspace + "/../etc/passwd")
		if err == nil {
			t.Error("expected error for path traversal")
		}
	})
}

func TestDeviceBlocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix-specific test")
	}
	sandbox := NewFilesystemSandbox("/tmp")
	err := sandbox.ValidateRead("/dev/sda")
	if err == nil {
		t.Error("expected error for device file")
	}
}
