//go:build windows
// +build windows

package winio

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestGetSymlinkReparsePoint(t *testing.T) {
	testDir := t.TempDir()
	testfilePath := filepath.Join(testDir, "test.txt")
	testfile, err := os.Create(testfilePath)
	if err != nil {
		t.Fatalf("failed to created test file: %s", err)
	}
	testfile.Close()

	linkDir := t.TempDir()
	linkfilePath := filepath.Join(linkDir, "link.txt")

	if err := os.Symlink(testfilePath, linkfilePath); err != nil {
		t.Fatalf("failed to create link: %s", err)
	}

	// retrieve reparse buffer for the link
	reparseBuffer, err := GetReparsePoint(linkfilePath)
	if err != nil {
		t.Fatalf("failed to get reparse point: %s", err)
	}

	rp, err := DecodeReparsePoint(reparseBuffer)
	if err != nil {
		t.Fatalf("failed to decode reparse point: %s", err)
	}

	if rp.IsMountPoint || !strings.EqualFold(rp.Target, testfilePath) {
		t.Fatalf("Reparse point doesn't match: %+v", rp)
	}
}

func TestGetDirectorySymlinkReparsePoint(t *testing.T) {
	testDir := t.TempDir()
	targetDirPath := filepath.Join(testDir, "test")
	if err := os.Mkdir(targetDirPath, 0777); err != nil {
		t.Fatalf("failed to created test directory: %s", err)
	}

	linkDir := t.TempDir()
	linkPath := filepath.Join(linkDir, "link")
	if err := os.Symlink(targetDirPath, linkPath); err != nil {
		t.Fatalf("failed to create link: %s", err)
	}

	// retrieve reparse buffer for the link
	reparseBuffer, err := GetReparsePoint(linkPath)
	if err != nil {
		t.Fatalf("failed to get reparse point: %s", err)
	}

	rp, err := DecodeReparsePoint(reparseBuffer)
	if err != nil {
		t.Fatalf("failed to decode reparse point: %s", err)
	}

	if rp.IsMountPoint || !strings.EqualFold(rp.Target, targetDirPath) {
		t.Fatalf("Reparse point doesn't match: %+v", rp)
	}
}

func TestSetReparsePoint(t *testing.T) {
	testDir := t.TempDir()
	testfilePath := filepath.Join(testDir, "test.txt")

	// If we use reparseTagMountPoint or reparseTagSymlink we need to
	// generate a fully valid ReparseDataBuffer struct. Instead we use
	// reparseTagWcifs and put some random data in the ReparesDataBuffer.
	reparseTagWcifs := 0x90001018
	r := ReparseDataBuffer{
		ReparseTag:        uint32(reparseTagWcifs),
		ReparseDataLength: 4,
		Reserved:          0,
		DataBuffer:        []byte{0xde, 0xad, 0xbe, 0xef},
	}

	utf16Path, err := windows.UTF16PtrFromString(testfilePath)
	if err != nil {
		t.Fatalf("failed to convert path into utf16: %s", err)
	}

	testfileHandle, err := windows.CreateFile(utf16Path, windows.GENERIC_WRITE|windows.GENERIC_READ, 0, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatalf("failed to open file handle: %s", err)
	}
	defer windows.Close(testfileHandle)

	if err := SetReparsePoint(testfileHandle, r.Encode()); err != nil {
		t.Fatalf("failed to set reparse point: %s", err)
	}
	windows.Close(testfileHandle)

	reparseBuffer, err := GetReparsePoint(testfilePath)
	if err != nil {
		t.Fatalf("failed to get reparse point: %s", err)
	}

	if bytes.Compare(reparseBuffer, r.Encode()) != 0 {
		t.Fatalf("retrieved reparse point buffer doesn't match")
	}
}
