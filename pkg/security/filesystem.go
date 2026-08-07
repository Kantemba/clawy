package security

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// FilesystemSandbox enforces directory-based access restrictions.
type FilesystemSandbox struct {
	allowedDirs   []string
	blockedPaths  []string
	allowSymlinks bool
	blockDevices  bool
}

// NewFilesystemSandbox creates a sandbox with the given allowed directories.
func NewFilesystemSandbox(allowedDirs ...string) *FilesystemSandbox {
	cleaned := make([]string, 0, len(allowedDirs))
	for _, dir := range allowedDirs {
		if abs, err := filepath.Abs(dir); err == nil {
			cleaned = append(cleaned, abs)
		}
	}
	return &FilesystemSandbox{
		allowedDirs:   cleaned,
		blockedPaths:  defaultBlockedPaths(),
		allowSymlinks: false,
		blockDevices:  true,
	}
}

// WithSymlinks allows or denies symlink resolution.
func (s *FilesystemSandbox) WithSymlinks(allow bool) *FilesystemSandbox {
	s.allowSymlinks = allow
	return s
}

// WithBlockDevices enables or disables device file blocking.
func (s *FilesystemSandbox) WithBlockDevices(block bool) *FilesystemSandbox {
	s.blockDevices = block
	return s
}

// AddBlockedPath adds a path pattern to the blocklist.
func (s *FilesystemSandbox) AddBlockedPath(pattern string) {
	s.blockedPaths = append(s.blockedPaths, pattern)
}

// ValidateRead checks if reading the given path is allowed.
func (s *FilesystemSandbox) ValidateRead(path string) error {
	return s.validatePath(path, false)
}

// ValidateWrite checks if writing the given path is allowed.
func (s *FilesystemSandbox) ValidateWrite(path string) error {
	return s.validatePath(path, true)
}

// ValidateDelete checks if deleting the given path is allowed.
func (s *FilesystemSandbox) ValidateDelete(path string) error {
	if err := s.validatePath(path, true); err != nil {
		return err
	}
	// Extra protection: never allow deletion of blocked paths.
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}
	for _, blocked := range s.blockedPaths {
		if strings.HasPrefix(abs, blocked) || abs == blocked {
			return &SecurityError{
				Category: "filesystem",
				Message:  "deletion of system path is not allowed: " + path,
			}
		}
	}
	return nil
}

func (s *FilesystemSandbox) validatePath(path string, isWrite bool) error {
	if path == "" {
		return &SecurityError{Category: "filesystem", Message: "empty path"}
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return &SecurityError{Category: "filesystem", Message: "invalid path: " + err.Error()}
	}

	// Resolve symlinks if not allowed.
	if !s.allowSymlinks {
		resolved, err := filepath.EvalSymlinks(abs)
		if err == nil && resolved != abs {
			abs = resolved
		}
	}

	// Check blocked paths.
	for _, blocked := range s.blockedPaths {
		if abs == blocked || strings.HasPrefix(abs, blocked+string(filepath.Separator)) {
			return &SecurityError{
				Category: "filesystem",
				Message:  "access to system path is not allowed: " + path,
			}
		}
	}

	// Check allowed directories.
	if len(s.allowedDirs) > 0 {
		allowed := false
		for _, dir := range s.allowedDirs {
			if abs == dir || strings.HasPrefix(abs, dir+string(filepath.Separator)) {
				allowed = true
				break
			}
		}
		if !allowed {
			return &SecurityError{
				Category: "filesystem",
				Message:  "path is outside allowed directories: " + path,
			}
		}
	}

	// Block device files on Unix.
	if s.blockDevices && runtime.GOOS != "windows" {
		if isDeviceFile(abs) {
			return &SecurityError{
				Category: "filesystem",
				Message:  "access to device files is not allowed: " + path,
			}
		}
	}

	return nil
}

// SafeCreateTemp creates a temporary file within the sandbox.
func (s *FilesystemSandbox) SafeCreateTemp(dir, pattern string) (*os.File, error) {
	if len(s.allowedDirs) > 0 {
		if err := s.ValidateWrite(dir); err != nil {
			return nil, err
		}
	}
	return os.CreateTemp(dir, pattern)
}

func isDeviceFile(path string) bool {
	return strings.HasPrefix(path, "/dev/") ||
		strings.HasPrefix(path, "/proc/") ||
		strings.HasPrefix(path, "/sys/")
}

func defaultBlockedPaths() []string {
	if runtime.GOOS == "windows" {
		return []string{
			`C:\Windows\System32`,
			`C:\Windows\SysWOW64`,
			`C:\Program Files`,
			`C:\Program Files (x86)`,
		}
	}
	return []string{
		"/etc/shadow",
		"/etc/passwd",
		"/etc/sudoers",
		"/root/.ssh",
		"/proc",
		"/sys",
	}
}
