package volmount

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/Microsoft/go-winio"
	"github.com/Microsoft/go-winio/vhd"
	"github.com/Microsoft/hcsshim/computestorage"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"
)

// fullObjectIDFromObjectID turns the given class and objectID into an objectID suitable for calling
// wmi.ExecWmiMethod on.
// There should be an __PATH property on the object for this, according to wbemtest.exe, but I cannot see
// how to get it from the objects exposed by wmi.
func fullObjectIDFromObjectID(class, objectID string) string {
	return class + `.ObjectId="` + strings.ReplaceAll(strings.ReplaceAll(objectID, `\`, `\\`), `"`, `\"`) + `"`
}

// createNTFSVHD creates a VHD formatted with NTFS of size `sizeGB` at the given `vhdPath`.
// Copied from "github.com/Microsoft/hcsshim/internal/hcs".CreateNTFSVHD
func createNTFSVHD(ctx context.Context, vhdPath string, sizeGB uint32) (err error) {
	if err := vhd.CreateVhdx(vhdPath, sizeGB, 1); err != nil {
		return errors.Wrap(err, "failed to create VHD")
	}

	vhd, err := vhd.OpenVirtualDisk(vhdPath, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		return errors.Wrap(err, "failed to open VHD")
	}
	defer func() {
		err2 := syscall.CloseHandle(vhd)
		if err == nil {
			err = errors.Wrap(err2, "failed to close VHD")
		}
	}()

	if err := computestorage.FormatWritableLayerVhd(ctx, windows.Handle(vhd)); err != nil {
		return errors.Wrap(err, "failed to format VHD")
	}

	return nil
}

func readReparsePoint(t *testing.T, path string) []byte {
	rpFile, err := winio.OpenForBackup(path, 0, 0, syscall.OPEN_EXISTING)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		closeErr := rpFile.Close()
		if closeErr != nil {
			// Assuming if we're already failing, failing more isn't wrong.
			t.Fatal(closeErr)
		}
	}()

	rdbbuf := make([]byte, syscall.MAXIMUM_REPARSE_DATA_BUFFER_SIZE)
	var bytesReturned uint32
	err = syscall.DeviceIoControl(syscall.Handle(rpFile.Fd()), syscall.FSCTL_GET_REPARSE_POINT, nil, 0, &rdbbuf[0], uint32(len(rdbbuf)), &bytesReturned, nil)
	if err != nil {
		t.Fatal(err)
	}

	return rdbbuf
}

func mountAtAndCheck(t *testing.T, volumePath, mountPoint string) {
	err := os.MkdirAll(mountPoint, 0)
	if err != nil {
		t.Fatal(err)
	}

	err = SetVolumeMountPoint(mountPoint, volumePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if t.Failed() {
			deleteErr := DeleteVolumeMountPoint(mountPoint)
			if deleteErr != nil {
				// Assuming if we're already failing, failing more isn't wrong.
				t.Fatal(deleteErr)
			}
		}
	}()

	mountPointVolumePath, err := GetVolumeNameForVolumeMountPoint(mountPoint)
	if err != nil {
		t.Fatal(err)
	}

	if mountPointVolumePath != volumePath {
		t.Fatalf("Mount read-back incorrectly, expected %s; got %s", volumePath, mountPointVolumePath)
	}

	rpBuff := readReparsePoint(t, mountPoint)

	rp, err := winio.DecodeReparsePoint(rpBuff)
	if err != nil {
		t.Fatal(err)
	}

	if !rp.IsMountPoint {
		t.Fatal("Mount point read as reparse point did not decode as mount point")
	}

	// volumePath starts with \\?\ but the reparse point data starts with \??\
	if rp.Target[0:4] != "\\??\\" || rp.Target[4:] != volumePath[4:] {
		t.Fatalf("Mount read as reparse point incorrectly, expected \\??\\%s; got %s", volumePath[4:], rp.Target)
	}
}

// TestVolumeMountAPIs creates and attaches a small VHD, and then exercises the
// various mount-point APIs against it.
func TestVolumeMountSuccess(t *testing.T) {

	dir, err := ioutil.TempDir("", "volmountapis")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	vhdPath := filepath.Join(dir, "small.vhdx")

	// Replication of "github.com/Microsoft/hcsshim/internal/hcs".CreateNTFSVHD
	err = createNTFSVHD(context.Background(), vhdPath, 1)
	if err != nil {
		if errno, ok := errors.Cause(err).(windows.Errno); ok && errno == windows.ERROR_PRIVILEGE_NOT_HELD {
			t.Skip(err)
		}
		t.Fatal(err)
	}
	defer os.Remove(vhdPath)

	vhdHandle, err := vhd.OpenVirtualDisk(vhdPath, vhd.VirtualDiskAccessNone, vhd.OpenVirtualDiskFlagNone)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.CloseHandle(vhdHandle)

	// This needs to be done elevated, or with some specific permission anyway.
	attachParams := vhd.AttachVirtualDiskParameters{Version: 2}
	err = vhd.AttachVirtualDisk(vhdHandle, vhd.AttachVirtualDiskFlagNoDriveLetter, &attachParams)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		detachErr := vhd.DetachVirtualDisk(vhdHandle)
		if detachErr != nil {
			// assuming if we're already failing, failing more isn't wrong
			t.Fatal(detachErr)
		}
	}()

	volumePath, err := computestorage.GetLayerVhdMountPath(context.Background(), windows.Handle(vhdHandle))
	if err != nil {
		t.Fatal(err)
	}

	// GetLayerVhdMountPath returns without the final \
	volumePath += "\\"

	mountPoints, err := GetMountPathsFromVolumeName(volumePath)
	if err != nil {
		t.Fatal(err)
	}

	if len(mountPoints) != 0 {
		t.Fatalf("Brand new volume was unexpectedly mounted at: %v", mountPoints)
	}

	mountPoint0 := filepath.Join(dir, "mount0")
	mountAtAndCheck(t, volumePath, mountPoint0)
	defer func() {
		deleteErr := DeleteVolumeMountPoint(mountPoint0)
		if deleteErr != nil {
			// Assuming if we're already failing, failing more isn't wrong.
			t.Fatal(deleteErr)
		}
	}()

	mountPoints, err = GetMountPathsFromVolumeName(volumePath)
	if err != nil {
		t.Fatal(err)
	}

	expectedMountPoints := []string{mountPoint0 + string(filepath.Separator)}
	require.ElementsMatch(t, expectedMountPoints, mountPoints, "Mount apparently failed, expected %v (order irrelevant): got %v", expectedMountPoints, mountPoints)

	mountPoint1 := filepath.Join(dir, "mount1")
	mountAtAndCheck(t, volumePath, mountPoint1)
	defer func() {
		deleteErr := DeleteVolumeMountPoint(mountPoint1)
		if deleteErr != nil {
			// Assuming if we're already failing, failing more isn't wrong.
			t.Fatal(deleteErr)
		}
	}()

	mountPoints, err = GetMountPathsFromVolumeName(volumePath)
	if err != nil {
		t.Fatal(err)
	}

	// No order guarantee on the mounts
	expectedMountPoints = []string{mountPoint0 + string(filepath.Separator), mountPoint1 + string(filepath.Separator)}
	require.ElementsMatch(t, expectedMountPoints, mountPoints, "Mount apparently failed, expected %v (order irrelevant): got %v", expectedMountPoints, mountPoints)

	// Note: DeleteVolumeMountPoint is tested in the defers above.
}
