//go:build windows

package osversion

import (
	"sync"

	"golang.org/x/sys/windows"
)

var (
	v    Version
	once sync.Once
)

// Get returns the Windows operating system version.
func Get() Version {
	once.Do(func() {
		vi := windows.RtlGetVersion()

		v.Major = MajorVersion(vi.MajorVersion)
		v.Minor = MinorVersion(vi.MinorVersion)
		v.Build = BuildNumber(vi.BuildNumber)
	})
	return v
}

// Build returns the Windows build number.
func Build() BuildNumber {
	return Get().Build
}
