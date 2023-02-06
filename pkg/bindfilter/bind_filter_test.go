//go:build windows
// +build windows

package bindfilter

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyFileBinding(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	fileName := "testFile.txt"
	srcFile := filepath.Join(source, fileName)
	dstFile := filepath.Join(destination, fileName)

	err := ApplyFileBinding(destination, source, false)
	if err != nil {
		t.Fatal(err)
	}
	defer removeFileBinding(t, destination)

	data := []byte("bind filter test")

	if err := os.WriteFile(srcFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	readData, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(readData) != string(data) {
		t.Fatalf("source and destination file contents differ. Expected: %s, got: %s", string(data), string(readData))
	}

	// Remove the file on the mount point. The mount is not read-only, this should work.
	if err := os.Remove(dstFile); err != nil {
		t.Fatalf("failed to remove file from mount point: %s", err)
	}

	// Check that it's gone from the source as well.
	if _, err := os.Stat(srcFile); err == nil {
		t.Fatalf("expected file %s to be gone but is not", srcFile)
	}
}

func removeFileBinding(t *testing.T, mountpoint string) {
	if err := RemoveFileBinding(mountpoint); err != nil {
		t.Logf("failed to remove file binding from %s: %q", mountpoint, err)
	}
}

func TestApplyFileBindingReadOnly(t *testing.T) {
	source := t.TempDir()
	destination := t.TempDir()
	fileName := "testFile.txt"
	srcFile := filepath.Join(source, fileName)
	dstFile := filepath.Join(destination, fileName)

	err := ApplyFileBinding(destination, source, true)
	if err != nil {
		t.Fatal(err)
	}
	defer removeFileBinding(t, destination)

	data := []byte("bind filter test")

	if err := os.WriteFile(srcFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	readData, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatal(err)
	}

	if string(readData) != string(data) {
		t.Fatalf("source and destination file contents differ. Expected: %s, got: %s", string(data), string(readData))
	}

	// Attempt to remove the file on the mount point
	err = os.Remove(dstFile)
	if err == nil {
		t.Fatalf("should not be able to remove a file from a read-only mount")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("expected an access denied error, got: %q", err)
	}
}

func TestEnsureOnlyOneTargetCanBeMounted(t *testing.T) {
	source := t.TempDir()
	secondarySource := t.TempDir()
	destination := t.TempDir()

	err := ApplyFileBinding(destination, source, false)
	if err != nil {
		t.Fatal(err)
	}

	defer removeFileBinding(t, destination)

	err = ApplyFileBinding(destination, secondarySource, false)
	if err == nil {
		removeFileBinding(t, destination)
		t.Fatalf("we should not be able to mount multiple targets in the same destination")
	}
}

func checkSourceIsMountedOnDestination(src, dst string) (bool, error) {
	mappings, err := GetBindMappings(dst)
	if err != nil {
		return false, err
	}

	found := false
	// There may be pre-existing mappings on the system.
	for _, mapping := range mappings {
		if mapping.MountPoint == dst {
			found = true
			if len(mapping.Targets) != 1 {
				return false, fmt.Errorf("expected only one target, got: %s", strings.Join(mapping.Targets, ", "))
			}
			if mapping.Targets[0] != src {
				return false, fmt.Errorf("expected target to be %s, got %s", src, mapping.Targets[0])
			}
			break
		}
	}

	return found, nil
}

func TestGetBindMappings(t *testing.T) {
	// GetBindMappings will exoand short paths like ADMINI~1 and PROGRA~1 to their
	// full names. In order to properly match the names later, we expand them here.
	srcShort := t.TempDir()
	source, err := getFinalPath(srcShort)
	if err != nil {
		t.Fatalf("failed to get long path")
	}

	dstShort := t.TempDir()
	destination, err := getFinalPath(dstShort)
	if err != nil {
		t.Fatalf("failed to get long path")
	}

	err = ApplyFileBinding(destination, source, false)
	if err != nil {
		t.Fatal(err)
	}
	defer removeFileBinding(t, destination)

	hasMapping, err := checkSourceIsMountedOnDestination(source, destination)
	if err != nil {
		t.Fatal(err)
	}

	if !hasMapping {
		t.Fatalf("expected to find %s mounted on %s, but could not", source, destination)
	}
}

func TestRemoveFileBinding(t *testing.T) {
	// GetBindMappings will exoand short paths like ADMINI~1 and PROGRA~1 to their
	// full names. In order to properly match the names later, we expand them here.
	srcShort := t.TempDir()
	source, err := getFinalPath(srcShort)
	if err != nil {
		t.Fatalf("failed to get long path")
	}

	dstShort := t.TempDir()
	destination, err := getFinalPath(dstShort)
	if err != nil {
		t.Fatalf("failed to get long path")
	}

	fileName := "testFile.txt"
	srcFile := filepath.Join(source, fileName)
	dstFile := filepath.Join(destination, fileName)
	data := []byte("bind filter test")

	if err := os.WriteFile(srcFile, data, 0600); err != nil {
		t.Fatal(err)
	}

	err = ApplyFileBinding(destination, source, false)
	if err != nil {
		t.Fatal(err)
	}
	defer removeFileBinding(t, destination)

	if _, err := os.Stat(dstFile); err != nil {
		t.Fatalf("expected to find %s, but did not", dstFile)
	}

	if err := RemoveFileBinding(destination); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dstFile); err == nil {
		t.Fatalf("expected %s to be gone, but it not", dstFile)
	}
}
