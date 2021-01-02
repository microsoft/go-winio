package volmount

import (
	"path/filepath"
	"syscall"
	"unicode/utf16"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// utf16ToStringArray returns the UTF-8 encoding of the sequence of UTF-16 sequences s,
// with a terminating NUL removed. The sequences are terminated by an additional NULL.
func utf16ToStringArray(s []uint16) ([]string, error) {
	var results []string
	prev := 0
	for i := range s {
		if s[i] == 0 {
			if prev == i {
				// found two null characters in a row, return result
				return results, nil
			}
			results = append(results, string(utf16.Decode(s[prev:i])))
			prev = i + 1
		}
	}
	return nil, errors.New("string set malformed: missing null terminator at end of buffer")
}

// GetMountPathsFromVolumeName returns a list of mount points for the volumePath
// (in format '\\?\Volume{GUID}).
// https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getvolumepathnamesforvolumenamew
func GetMountPathsFromVolumeName(volumePath string) ([]string, error) {
	// Must end in a backslash
	slashedVolume := filepath.Clean(volumePath)
	if slashedVolume[len(slashedVolume)-1] != filepath.Separator {
		slashedVolume = slashedVolume + string(filepath.Separator)
	}

	volumeP, err := windows.UTF16PtrFromString(slashedVolume)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to utf16-ise %s", slashedVolume)
	}

	var bufferLength uint32
	err = windows.GetVolumePathNamesForVolumeName(volumeP, nil, 0, &bufferLength)
	if err == nil {
		// This should never happen. An empty list would have a single 0 in it.
		return nil, errors.Errorf("unexpected success of GetVolumePathNamesForVolumeName('%s', nil, 0, ...)", slashedVolume)
	} else if err != syscall.ERROR_MORE_DATA {
		return nil, errors.Wrapf(err, "failed calling GetVolumePathNamesForVolumeName('%s', nil, 0, ...)", slashedVolume)
	}

	buffer := make([]uint16, bufferLength)
	err = windows.GetVolumePathNamesForVolumeName(volumeP, &buffer[0], bufferLength, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed calling GetVolumePathNamesForVolumeName('%s', ..., %d, nil)", slashedVolume, bufferLength)
	}

	result, err := utf16ToStringArray(buffer)
	if err != nil {
		return nil, errors.Wrapf(err, "failed decoding result of GetVolumePathNamesForVolumeName('%s', ..., %d, nil)", slashedVolume, bufferLength)
	}

	return result, nil
}
