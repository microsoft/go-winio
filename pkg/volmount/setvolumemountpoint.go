package volmount

import (
	"path/filepath"
	"strings"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// SetVolumeMountPoint mounts volumePath (in format '\\?\Volume{GUID}' at targetPath.
// https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-setvolumemountpointw
func SetVolumeMountPoint(targetPath string, volumePath string) error {
	if !strings.HasPrefix(volumePath, "\\\\?\\Volume{") {
		return errors.Errorf("unable to mount non-volume path %s", volumePath)
	}

	// Both must end in a backslash
	slashedTarget := filepath.Clean(targetPath)
	if slashedTarget[len(slashedTarget)-1] != filepath.Separator {
		slashedTarget = slashedTarget + string(filepath.Separator)
	}
	slashedVolume := filepath.Clean(volumePath)
	if slashedVolume[len(slashedVolume)-1] != filepath.Separator {
		slashedVolume = slashedVolume + string(filepath.Separator)
	}

	targetP, err := windows.UTF16PtrFromString(slashedTarget)
	if err != nil {
		return errors.Wrapf(err, "unable to utf16-ise %s", slashedTarget)
	}

	volumeP, err := windows.UTF16PtrFromString(slashedVolume)
	if err != nil {
		return errors.Wrapf(err, "unable to utf16-ise %s", slashedVolume)
	}

	if err := windows.SetVolumeMountPoint(targetP, volumeP); err != nil {
		return errors.Wrapf(err, "failed calling SetVolumeMount('%s', '%s')", slashedTarget, slashedVolume)
	}

	return nil
}
