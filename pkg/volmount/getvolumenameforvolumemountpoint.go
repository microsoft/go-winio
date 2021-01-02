package volmount

import (
	"path/filepath"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// GetVolumeNameForVolumeMountPoint returns a volume path (in format '\\?\Volume{GUID}'
// for the volume mounted at targetPath.
// https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getvolumenameforvolumemountpointw
func GetVolumeNameForVolumeMountPoint(targetPath string) (string, error) {
	// Must end in a backslash
	slashedTarget := filepath.Clean(targetPath)
	if slashedTarget[len(slashedTarget)-1] != filepath.Separator {
		slashedTarget = slashedTarget + string(filepath.Separator)
	}

	targetP, err := windows.UTF16PtrFromString(slashedTarget)
	if err != nil {
		return "", errors.Wrapf(err, "unable to utf16-ise %s", slashedTarget)
	}

	bufferlength := uint32(50) // "A reasonable size for the buffer" per the documentation.
	buffer := make([]uint16, bufferlength)

	if err = windows.GetVolumeNameForVolumeMountPoint(targetP, &buffer[0], bufferlength); err != nil {
		return "", errors.Wrapf(err, "failed calling GetVolumeNameForVolumeMountPoint('%s', ..., %d)", slashedTarget, bufferlength)
	}

	return windows.UTF16ToString(buffer), nil
}
