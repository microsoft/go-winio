package osversion

import (
	"fmt"
)

// Documentation and [OSVERSIONINFO] struct lists fields as DWORDs (uint32), but they
// all packed into a uint32 in GetVersion, so safe to downcast to smallper types
//
//[OSVERSIONINFO]: https://learn.microsoft.com/en-us/windows/win32/api/winnt/ns-winnt-osversioninfoexw

type (
	MajorVersion uint8
	MinorVersion uint8
	BuildNumber  uint16
)

// Version is a wrapper for Windows version information
//
// https://learn.microsoft.com/en-us/windows/win32/api/sysinfoapi/nf-sysinfoapi-getversion
type Version struct {
	Major MajorVersion
	Minor MinorVersion
	Build BuildNumber
}

func FromPackedVersion(v uint32) Version {
	return Version{
		Major: MajorVersion(v & 0xFF),
		Minor: MinorVersion(v >> 8 & 0xFF),
		Build: BuildNumber(v >> 16),
	}
}

var _ fmt.Stringer = Version{}

// String returns the OSVersion formatted as a string.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Build)
}

// Compare compares the current OSVersion to another.
// The result will be 0 if they are equal, -1 the current version is lesser, and +1 otherwise.
func (v Version) Compare(other Version) int {
	cmp := func(a, b int) int {
		if a > b {
			return 1
		} else if a < b {
			return -1
		}
		return 0
	}

	if c := cmp(int(v.Major), int(other.Major)); c != 0 {
		return c
	}
	if c := cmp(int(v.Minor), int(other.Minor)); c != 0 {
		return c
	}
	return cmp(int(v.Build), int(other.Build))
}
