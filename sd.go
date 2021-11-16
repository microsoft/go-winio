//go:build windows
// +build windows

package winio

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

//sys lookupAccountName(systemName *uint16, accountName string, sid *byte, sidSize *uint32, refDomain *uint16, refDomainSize *uint32, sidNameUse *uint32) (err error) = advapi32.LookupAccountNameW
//sys convertSidToStringSid(sid *byte, str **uint16) (err error) = advapi32.ConvertSidToStringSidW
//sys convertStringSecurityDescriptorToSecurityDescriptor(str string, revision uint32, sd *uintptr, size *uint32) (err error) = advapi32.ConvertStringSecurityDescriptorToSecurityDescriptorW
//sys convertSecurityDescriptorToStringSecurityDescriptor(sd *byte, revision uint32, secInfo uint32, sddl **uint16, sddlSize *uint32) (err error) = advapi32.ConvertSecurityDescriptorToStringSecurityDescriptorW
//sys localFree(mem uintptr) = LocalFree
//sys getSecurityDescriptorLength(sd uintptr) (len uint32) = advapi32.GetSecurityDescriptorLength

const (
	cERROR_NONE_MAPPED = syscall.Errno(1332)
)

type AccountLookupError struct {
	Name string
	Err  error
}

func (e *AccountLookupError) Error() string {
	if e.Name == "" {
		return "lookup account: empty account name specified"
	}
	var s string
	switch e.Err {
	case cERROR_NONE_MAPPED:
		s = "not found"
	default:
		s = e.Err.Error()
	}
	return "lookup account " + e.Name + ": " + s
}

type SddlConversionError struct {
	Sddl string
	Err  error
}

func (e *SddlConversionError) Error() string {
	return "convert " + e.Sddl + ": " + e.Err.Error()
}

// LookupSidByName looks up the SID of an account by name
func LookupSidByName(name string) (sid string, err error) {
	if name == "" {
		return "", &AccountLookupError{name, cERROR_NONE_MAPPED}
	}

	var sidSize, sidNameUse, refDomainSize uint32
	err = lookupAccountName(nil, name, nil, &sidSize, nil, &refDomainSize, &sidNameUse)
	if err != nil && err != syscall.ERROR_INSUFFICIENT_BUFFER {
		return "", &AccountLookupError{name, err}
	}
	sidBuffer := make([]byte, sidSize)
	refDomainBuffer := make([]uint16, refDomainSize)
	err = lookupAccountName(nil, name, &sidBuffer[0], &sidSize, &refDomainBuffer[0], &refDomainSize, &sidNameUse)
	if err != nil {
		return "", &AccountLookupError{name, err}
	}
	var strBuffer *uint16
	err = convertSidToStringSid(&sidBuffer[0], &strBuffer)
	if err != nil {
		return "", &AccountLookupError{name, err}
	}
	sid = syscall.UTF16ToString((*[0xffff]uint16)(unsafe.Pointer(strBuffer))[:])
	localFree(uintptr(unsafe.Pointer(strBuffer)))
	return sid, nil
}

func SddlToSecurityDescriptor(sddl string) ([]byte, error) {
	var sdBuffer uintptr
	err := convertStringSecurityDescriptorToSecurityDescriptor(sddl, 1, &sdBuffer, nil)
	if err != nil {
		return nil, &SddlConversionError{sddl, err}
	}
	defer localFree(sdBuffer)
	sd := make([]byte, getSecurityDescriptorLength(sdBuffer))
	copy(sd, (*[0xffff]byte)(unsafe.Pointer(sdBuffer))[:len(sd)])
	return sd, nil
}

func SecurityDescriptorToSddl(sd []byte) (string, error) {
	var sddl *uint16
	// The returned string length seems to include an arbitrary number of terminating NULs.
	// Don't use it.
	err := convertSecurityDescriptorToStringSecurityDescriptor(&sd[0], 1, 0xff, &sddl, nil)
	if err != nil {
		return "", err
	}
	defer localFree(uintptr(unsafe.Pointer(sddl)))
	return syscall.UTF16ToString((*[0xffff]uint16)(unsafe.Pointer(sddl))[:]), nil
}

func GetFileSecurityDescriptor(path string) (*windows.SECURITY_DESCRIPTOR, error) {
	utf16Path, err := windows.UTF16FromString(path)
	if err != nil {
		return nil, err
	}
	fileHandle, err := windows.CreateFile(&utf16Path[0], (windows.READ_CONTROL | windows.ACCESS_SYSTEM_SECURITY), 0, nil, windows.OPEN_EXISTING, (windows.FILE_ATTRIBUTE_NORMAL | windows.FILE_FLAG_BACKUP_SEMANTICS), 0)
	if err != nil {
		return nil, fmt.Errorf("open file failed with: %w", err)
	}
	sd, err := windows.GetSecurityInfo(fileHandle, windows.SE_FILE_OBJECT, (windows.ATTRIBUTE_SECURITY_INFORMATION | windows.DACL_SECURITY_INFORMATION | windows.GROUP_SECURITY_INFORMATION | windows.LABEL_SECURITY_INFORMATION | windows.OWNER_SECURITY_INFORMATION | windows.SACL_SECURITY_INFORMATION | windows.SCOPE_SECURITY_INFORMATION | windows.BACKUP_SECURITY_INFORMATION))
	if err != nil {
		return nil, fmt.Errorf("get security info failed: %w", err)
	}
	return sd, nil
}
