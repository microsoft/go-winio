package fs

import (
	"errors"
	"io/fs"
	"unsafe"

	"golang.org/x/sys/windows"
)

// We want to use findFirstFileExW since it allows us to avoid the shortname lookup.
// Package golang.org/x/sys/windows only defines FindFirstFile, so we define our own here.
// We could use its findNextFileW definition, except that it also has an inaccurate version of Win32finddata[1].
// Therefore we define our own versions of both, as well as our own findData struct.
//
// [1] This inaccurate version is still usable, as the name and alternate name fields are one character short,
// but the original value is guaranteed to be null terminated, so you just lose the null terminator in the worst case.
// However, this is still annoying to deal with, as you either must find the null in the array, or treat the full array
// as a null terminated string.

//sys findFirstFileExW(pattern *uint16, infoLevel uint32, data *findData, searchOp uint32, searchFilter unsafe.Pointer, flags uint32) (h windows.Handle, err error) [failretval==windows.InvalidHandle] = kernel32.FindFirstFileExW
//sys findNextFileW(findHandle windows.Handle, data *findData) (err error) = kernel32.FindNextFileW

const (
	// Part of FINDEX_INFO_LEVELS.
	//
	// https://learn.microsoft.com/en-us/windows/win32/api/minwinbase/ne-minwinbase-findex_info_levels
	findExInfoBasic = 1

	// Part of FINDEX_SEARCH_OPS.
	//
	// https://learn.microsoft.com/en-us/windows/win32/api/minwinbase/ne-minwinbase-findex_search_ops
	findExSearchNameMatch = 0

	// Indicates the reparse point is a name surrogate, which means it refers to another filesystem entry.
	// This is used for things like symlinks and junction (mount) points.
	//
	// https://learn.microsoft.com/en-us/windows/win32/fileio/reparse-point-tags#tag-contents
	reparseTagNameSurrogate = 0x20000000
)

// findData represents Windows's WIN32_FIND_DATAW type.
// The obsolete items at the end of the struct in the docs are not actually present, except on Mac.
//
// https://learn.microsoft.com/en-us/windows/win32/api/minwinbase/ns-minwinbase-win32_find_dataw
type findData struct {
	attributes    uint32
	creationTime  windows.Filetime
	accessTime    windows.Filetime
	writeTime     windows.Filetime
	fileSizeHigh  uint32
	fileSizeLow   uint32
	reserved0     uint32 // Holds reparse point tag when file is a reparse point.
	reserved1     uint32
	name          [windows.MAX_PATH]uint16
	alternateName [14]uint16
}

// fileAttributeTagInfo represents Windows's FILE_ATTRIBUTE_TAG_INFO type.
//
// https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_attribute_tag_infof
type fileAttributeTagInfo struct {
	attributes uint32
	tag        uint32
}

var (
	// Mockable for testing.
	removeDirectory = windows.RemoveDirectory
)

// RemoveAll attempts to be a Windows-specific replacement for os.RemoveAll.
// Specifically, it treats a filesystem path as a []uint16 rather than a string. This allows it to work in some cases
// where a string-based API would not, such as when a filesystem name is not valid UTF-16 (e.g. low surrogates without preceding high surrogates).
//
// RemoveAll handles some dynamic changes to the directory tree while we are deleting it. For instance,
// if a directory is not empty, we delete its children first, and then try again. If new items are added to the
// directory while we do that, we will again recurse and delete them, and keep trying. This matches the behavior of
// os.RemoveAll.
// However, we don't attempt to handle every dynamic case, for instance:
//   - If a file has FILE_ATTRIBUTE_READONLY re-set on it after we clear it, we will fail.
//   - If an entry is changed from file to directory after we query its attributes, we will fail.
func RemoveAll(path []uint16) error {
	attrs, reparseTag, err := getFileInfo(path)
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return nil
	} else if err != nil {
		return err
	}
	return removeAll(path, attrs, reparseTag)
}

// removeAll is the lower-level routine for recursively deleting a file system entry.
// The attributes and reparse point of the top-level item to be deleted must have already been queried and are supplied as an argument.
func removeAll(path []uint16, attrs uint32, reparseTag uint32) error {
	if attrs&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
		// File
		if attrs&windows.FILE_ATTRIBUTE_READONLY != 0 {
			// Read-only flag prevents deletion, so un-set it first.
			if err := windows.SetFileAttributes(terminate(path), attrs&^windows.FILE_ATTRIBUTE_READONLY); err == windows.ERROR_FILE_NOT_FOUND {
				return nil
			} else if err != nil {
				return pathErr("SetFileAttributes", path, err)
			}
		}
		if err := windows.DeleteFile(terminate(path)); err == windows.ERROR_FILE_NOT_FOUND {
			return nil
		} else if err != nil {
			return pathErr("DeleteFile", path, err)
		}
	} else {
		// Directory
		// Keep looping, removing children, and attempting to delete again.
		for {
			// First, try to delete the directory. This will only work if it's empty.
			// If that fails then enumerate the entries and delete them first.
			if err := removeDirectory(terminate(path)); err == nil || err == windows.ERROR_FILE_NOT_FOUND {
				return nil
			} else if err != windows.ERROR_DIR_NOT_EMPTY || (attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 && reparseTag&reparseTagNameSurrogate != 0) {
				// We either failed for some reason other than the directory not being empty, or because the directory is a name surrogate reparse point.
				// We don't want to recurse into a name surrogate (e.g. symlink) because we will end up deleting stuff elsewhere on the system.
				// In this case there's nothing else to do for this entry, so just return an error.
				return pathErr("RemoveDirectory", path, err)
			}
			// Wrapping in an anonymous function so that the deferred FindClose call is scoped properly.
			// Iterate the directory and remove all children by recursing into removeAll.
			if err := func() error {
				var fd findData
				// findFirstFileExW allows us to avoid the shortname lookup. See comment at top of file for details.
				pattern := join(path, []uint16{'*'})
				find, err := findFirstFileExW(terminate(pattern), findExInfoBasic, &fd, findExSearchNameMatch, nil, 0)
				if err == windows.ERROR_FILE_NOT_FOUND {
					// No children is weird, because we should always have "." and "..".
					// If we do hit this, just continue to the next deletion attempt.
					return nil
				} else if err != nil {
					return pathErr("FindFirstFileEx", pattern, err)
				}
				defer windows.FindClose(find)
				for {
					var child []uint16
					child, err = truncAtNull(fd.name[:])
					if err != nil {
						return err
					}
					if !equal(child, []uint16{'.'}) && !equal(child, []uint16{'.', '.'}) { // Ignore "." and ".."
						if err := removeAll(join(path, child), fd.attributes, fd.reserved0); err != nil {
							return err
						}
					}
					err = findNextFileW(find, &fd)
					if err == windows.ERROR_NO_MORE_FILES {
						break
					} else if err != nil {
						return pathErr("FindNextFile", path, err)
					}
				}
				return nil
			}(); err != nil {
				return err
			}
		}
	}
	return nil
}

func getFileInfo(path []uint16) (attrs uint32, reparseTag uint32, _ error) {
	h, err := windows.CreateFile(terminate(path), 0, 0, nil, windows.OPEN_EXISTING, windows.FILE_OPEN_REPARSE_POINT|windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return 0, 0, pathErr("CreateFile", path, err)
	}
	defer windows.CloseHandle(h) //nolint:errcheck
	var ti fileAttributeTagInfo
	if err = windows.GetFileInformationByHandleEx(h, windows.FileAttributeTagInfo, (*byte)(unsafe.Pointer(&ti)), uint32(unsafe.Sizeof(ti))); err != nil {
		return 0, 0, pathErr("GetFileInformationByHandleEx", path, err)
	}
	return ti.attributes, ti.tag, nil
}

func equal(v1, v2 []uint16) bool {
	if len(v1) != len(v2) {
		return false
	}
	for i := range v1 {
		if v1[i] != v2[i] {
			return false
		}
	}
	return true
}

func join(parent, child []uint16) []uint16 {
	return append(append(parent, '\\'), child...)
}

func pathErr(op string, path []uint16, err error) error {
	return &fs.PathError{Op: op, Path: windows.UTF16ToString(path), Err: err}
}

// terminate takes a []uint16 and returns a null-terminated *uint16.
func terminate(path []uint16) *uint16 {
	return &append(path, '\u0000')[0]
}

// truncAtNull searches the input for a null terminator and returns a slice
// up to that point. It returns an error if there is no null terminator in the input.
func truncAtNull(path []uint16) ([]uint16, error) {
	for i, u := range path {
		if u == '\u0000' {
			return path[:i], nil
		}
	}
	return nil, errors.New("path is not null terminated")
}
