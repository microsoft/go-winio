//go:build windows

package vhd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Microsoft/go-winio/pkg/guid"
)

func TestVirtualDiskIdentifier(t *testing.T) {
	// Create a temporary directory for the test
	tempDir := t.TempDir()
	// TODO(ambarve): We should add a test for VHD too, but our current create VHD API
	// seem to only work for VHDX.
	vhdPath := filepath.Join(tempDir, "test.vhdx")

	// Create the virtual disk
	if err := CreateVhdx(vhdPath, 1, 1); err != nil { // 1GB, 1MB block size
		t.Fatalf("failed to create virtual disk: %s", err)
	}
	defer os.Remove(vhdPath)

	// Get the initial identifier
	initialID, err := GetVirtualDiskIdentifier(vhdPath)
	if err != nil {
		t.Fatalf("failed to get initial virtual disk identifier: %s", err)
	}
	t.Logf("initial identifier: %s", initialID.String())

	// Create a new GUID to set
	newID := guid.GUID{
		Data1: 0x12345678,
		Data2: 0x1234,
		Data3: 0x5678,
		Data4: [8]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0},
	}
	t.Logf("setting new identifier: %s", newID.String())

	// Set the new identifier
	if err := SetVirtualDiskIdentifier(vhdPath, newID); err != nil {
		t.Fatalf("failed to set virtual disk identifier: %s", err)
	}

	// Get the identifier again to verify it was set correctly
	retrievedID, err := GetVirtualDiskIdentifier(vhdPath)
	if err != nil {
		t.Fatalf("failed to get virtual disk identifier after setting: %s", err)
	}
	t.Logf("retrieved identifier: %s", retrievedID.String())

	// Verify the retrieved ID matches the one we set
	if retrievedID != newID {
		t.Errorf("retrieved identifier does not match set identifier.\nExpected: %s\nGot: %s",
			newID.String(), retrievedID.String())
	}
}
