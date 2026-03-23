//go:build windows

package winio

import (
	"testing"
)

const (
	testLxSymlinkAbsolutePath     = "/usr/bin/bash"
	testWindowsSymlinkPath        = `C:\Windows\System32`
	testLxSymlinkRelativePath     = "../bin/sh"
	testLxSymlinkSpecialCharsPath = "/path/with spaces/and-special!@#$%/файл.txt"
)

func TestLxSymlinkRoundTrip(t *testing.T) {
	// Test LX symlink encode/decode
	original := &ReparsePoint{
		Target:       testLxSymlinkAbsolutePath,
		IsMountPoint: false,
		IsLxSymlink:  true,
	}

	// Encode
	encoded := EncodeReparsePoint(original)

	// Decode
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	// Verify
	if decoded.Target != original.Target {
		t.Errorf("Target mismatch: got %q, want %q", decoded.Target, original.Target)
	}
	if decoded.IsLxSymlink != original.IsLxSymlink {
		t.Errorf("IsLxSymlink mismatch: got %v, want %v", decoded.IsLxSymlink, original.IsLxSymlink)
	}
	if decoded.IsMountPoint != original.IsMountPoint {
		t.Errorf("IsMountPoint mismatch: got %v, want %v", decoded.IsMountPoint, original.IsMountPoint)
	}
}

func TestWindowsSymlinkNotLx(t *testing.T) {
	// Test that regular Windows symlinks are not marked as LX
	original := &ReparsePoint{
		Target:       testWindowsSymlinkPath,
		IsMountPoint: false,
		IsLxSymlink:  false,
	}

	// Encode
	encoded := EncodeReparsePoint(original)

	// Decode
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	// Verify it's NOT an LX symlink
	if decoded.IsLxSymlink {
		t.Error("Windows symlink incorrectly marked as LX symlink")
	}
}

func TestLxSymlinkEmptyTarget(t *testing.T) {
	// Test LX symlink with empty target
	original := &ReparsePoint{
		Target:       "",
		IsMountPoint: false,
		IsLxSymlink:  true,
	}

	// Encode
	encoded := EncodeReparsePoint(original)

	// Decode
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	// Verify
	if decoded.Target != original.Target {
		t.Errorf("Target mismatch: got %q, want %q", decoded.Target, original.Target)
	}
	if !decoded.IsLxSymlink {
		t.Error("IsLxSymlink should be true")
	}
}

func TestLxSymlinkRelativePath(t *testing.T) {
	// Test LX symlink with relative path
	original := &ReparsePoint{
		Target:       testLxSymlinkRelativePath,
		IsMountPoint: false,
		IsLxSymlink:  true,
	}

	// Encode
	encoded := EncodeReparsePoint(original)

	// Decode
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	// Verify
	if decoded.Target != original.Target {
		t.Errorf("Target mismatch: got %q, want %q", decoded.Target, original.Target)
	}
	if !decoded.IsLxSymlink {
		t.Error("IsLxSymlink should be true")
	}
}

func TestLxSymlinkSpecialCharacters(t *testing.T) {
	// Test LX symlink with special characters and Unicode
	original := &ReparsePoint{
		Target:       testLxSymlinkSpecialCharsPath,
		IsMountPoint: false,
		IsLxSymlink:  true,
	}

	// Encode
	encoded := EncodeReparsePoint(original)

	// Decode
	decoded, err := DecodeReparsePoint(encoded)
	if err != nil {
		t.Fatalf("Failed to decode: %v", err)
	}

	// Verify
	if decoded.Target != original.Target {
		t.Errorf("Target mismatch: got %q, want %q", decoded.Target, original.Target)
	}
	if !decoded.IsLxSymlink {
		t.Error("IsLxSymlink should be true")
	}
}

func TestEncodeReparsePointNil(t *testing.T) {
	// Test encoding a nil ReparsePoint
	encoded := EncodeReparsePoint(nil)
	if encoded != nil {
		t.Errorf("Expected nil result for nil input, got %v", encoded)
	}
}
