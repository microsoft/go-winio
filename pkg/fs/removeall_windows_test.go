package fs

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

type fsEntry interface {
	create(parent []uint16) error
}

type file struct {
	name     string
	rawName  []uint16
	readOnly bool
}

func (f file) create(parent []uint16) error {
	if f.name != "" && f.rawName != nil {
		return errors.New("cannot set both name and rawName")
	}
	name := f.rawName
	if f.name != "" {
		name = utf16.Encode([]rune(f.name))
	}
	p := join(parent, name)
	var attrs uint32
	if f.readOnly {
		attrs |= windows.FILE_ATTRIBUTE_READONLY
	}
	h, err := windows.CreateFile(
		terminate(p),
		windows.GENERIC_ALL,
		0,
		nil,
		windows.CREATE_NEW,
		attrs,
		0)
	if err != nil {
		return pathErr("CreateFile", p, err)
	}
	return windows.CloseHandle(h)
}

type dir struct {
	name     string
	rawName  []uint16
	children []fsEntry
}

func (d dir) create(parent []uint16) error {
	if d.name != "" && d.rawName != nil {
		return errors.New("cannot set both name and rawName")
	}
	name := d.rawName
	if d.name != "" {
		name = utf16.Encode([]rune(d.name))
	}
	p := join(parent, name)
	if err := windows.CreateDirectory(terminate(p), nil); err != nil {
		return pathErr("CreateDirectory", p, err)
	}
	for _, c := range d.children {
		if err := c.create(p); err != nil {
			return err
		}
	}
	return nil
}

// We use junctions instead of symlinks because CreateSymbolicLink requires either
// Administrator privileges or  Developer Mode enabled, which are both annoying to
// require for test code to run.
type junction struct {
	name   string
	target string
}

func (j junction) create(parent []uint16) error {
	// There isn't a simple Windows API to create a junction, so we shell out for mklink instead.
	// The alternative would be manually creating the reparse point buffer and calling
	// the fsctl, which is too annoying for test code.
	p := filepath.Join(windows.UTF16ToString(parent), j.name)
	c := exec.Command("cmd.exe", "/c", "mklink", "/J", p, j.target)
	if err := c.Run(); err != nil {
		return &os.LinkError{Op: "mklink", New: p, Old: j.target, Err: err}
	}
	return nil
}

// TestRemoveAll creates a series of nested filesystem entries (files, directories, and junctions) beneath a temp root,
// then calls RemoveAll on the root, then tests to ensure the contents were deleted.
func TestRemoveAll(t *testing.T) {
	root := t.TempDir()
	t.Logf("Root directory: %s", root)
	rootU16 := utf16.Encode([]rune(root))

	entries := []fsEntry{
		dir{name: "dir", children: []fsEntry{
			dir{name: "childdir", children: []fsEntry{
				file{name: "bar.txt"},
			}},
			file{name: "baz.txt"},
		}},
		dir{name: "emptydir"},
		dir{name: "fakeemptydir", children: []fsEntry{
			file{name: "thisfilewillbedeleted"},
		}},
		file{name: "foo.txt"},
		// This file name was seen in a real case where os.RemoveAll failed. It is invalid UTF-16 as it contains low surrogates that are not preceded by high surrogates ([1:5]).
		file{rawName: []uint16{0x2e, 0xdc6d, 0xdc73, 0xdc79, 0xdc73, 0x30, 0x30, 0x30, 0x31, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x30, 0x35, 0x36, 0x33, 0x39, 0x64, 0x64, 0x35, 0x30, 0x61, 0x37, 0x32, 0x61, 0x62, 0x34, 0x36, 0x36, 0x38, 0x62, 0x33, 0x33}},
		file{name: "readonlyfile", readOnly: true},
	}
	for _, entry := range entries {
		if err := entry.create(rootU16); err != nil {
			t.Fatal(err)
		}
	}

	if err := RemoveAll(rootU16); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Errorf("root dir exists when it should not: %s", root)
	}
}

// TestRemoveAllDontFollowSymlinks creates a junction pointing to another directory, then calls RemoveAll on it, then tests
// to ensure the referenced directory or its contents were not deleted.
func TestRemoveAllDontFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	t.Logf("Root directory: %s", root)
	rootU16 := utf16.Encode([]rune(root))
	junctionDir := t.TempDir()
	t.Logf("Junction directory: %s", junctionDir)
	// We will later test to ensure fileinjunction still exists.
	if err := (file{name: "fileinjunction"}.create(utf16.Encode([]rune(junctionDir)))); err != nil {
		t.Fatal(err)
	}

	entries := []fsEntry{
		junction{name: "link", target: junctionDir},
	}
	for _, entry := range entries {
		if err := entry.create(rootU16); err != nil {
			t.Fatal(err)
		}
	}

	if err := RemoveAll(rootU16); err != nil {
		t.Errorf("RemoveAll failed: %s", err)
	}

	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Errorf("root dir exists when it should not: %s", root)
	}
	if _, err := os.Lstat(junctionDir); err != nil {
		t.Errorf("junction dir may have been deleted when it should not be: %s", err)
	}
	if _, err := os.Lstat(filepath.Join(junctionDir, "fileinjunction")); err != nil {
		t.Errorf("file in junction dir may have been deleted when it should not be: %s", err)
	}
}

// TestRemoveAllShouldFailWhenSymlinkDeletionFails creates a junction pointing to another directory, then calls RemoveAll on it, then tests
// to ensure the referenced directory or its contents were not deleted. However in this case we mock the test so that RemoveDirectory fails
// the first time on the junction. This is to ensure we don't accidentally recurse into the symlink when this happens.
func TestRemoveAllShouldFailWhenSymlinkDeletionFails(t *testing.T) {
	root := t.TempDir()
	t.Logf("Root directory: %s", root)
	rootU16 := utf16.Encode([]rune(root))
	junctionDir := t.TempDir()
	t.Logf("Junction directory: %s", junctionDir)
	// We will later test to ensure fileinjunction still exists.
	if err := (file{name: "fileinjunction"}.create(utf16.Encode([]rune(junctionDir)))); err != nil {
		t.Fatal(err)
	}

	entries := []fsEntry{
		junction{name: "link", target: junctionDir},
	}
	for _, entry := range entries {
		if err := entry.create(rootU16); err != nil {
			t.Errorf("RemoveAll failed: %s", err)
		}
	}

	var linkDeleteAttempted bool
	removeDirectory = func(path *uint16) error {
		if _, name := filepath.Split(windows.UTF16PtrToString(path)); !linkDeleteAttempted && name == "link" {
			linkDeleteAttempted = true
			return windows.ERROR_DIR_NOT_EMPTY
		}
		return windows.RemoveDirectory(path)
	}

	if err := RemoveAll(rootU16); err != nil {
		// We expect RemoveAll to return an error as it failed to delete the link.
		pathErr, ok := err.(*fs.PathError)
		if !ok {
			t.Errorf("RemoveAll failed: %s", err)
		}
		if _, name := filepath.Split(pathErr.Path); pathErr.Op != "RemoveDirectory" || pathErr.Err != windows.ERROR_DIR_NOT_EMPTY || name != "link" {
			t.Errorf("RemoveAll failed: %s", err)
		}
	}

	if _, err := os.Lstat(junctionDir); err != nil {
		t.Errorf("junction dir may have been deleted when it should not be: %s", err)
	}
	if _, err := os.Lstat(filepath.Join(junctionDir, "fileinjunction")); err != nil {
		t.Errorf("file in junction dir may have been deleted when it should not be: %s", err)
	}
}
