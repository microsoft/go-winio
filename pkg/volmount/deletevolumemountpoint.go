package volmount

import (
	"path/filepath"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// DeleteVolumeMountPoint removes the volume mount at targetPath
// https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-deletevolumemountpointa
func DeleteVolumeMountPoint(targetPath string) error {
	// Must end in a backslash
	slashedTarget := filepath.Clean(targetPath)
	if slashedTarget[len(slashedTarget)-1] != filepath.Separator {
		slashedTarget = slashedTarget + string(filepath.Separator)
	}

	targetP, err := windows.UTF16PtrFromString(slashedTarget)
	if err != nil {
		return errors.Wrapf(err, "unable to utf16-ise %s", slashedTarget)
	}

	if err := windows.DeleteVolumeMountPoint(targetP); err != nil {
		return errors.Wrapf(err, "failed calling DeleteVolumeMountPoint('%s')", slashedTarget)
	}

	return nil
}
