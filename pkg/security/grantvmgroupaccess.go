package security

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/pkg/errors"
)

type (
	accessMask          uint32
	accessMode          uint32
	desiredAccess       uint32
	inheritMode         uint32
	objectType          uint32
	shareMode           uint32
	securityInformation uint32
	trusteeForm         uint32
	trusteeType         uint32

	explicitAccess struct {
		accessPermissions accessMask
		accessMode        accessMode
		inheritance       inheritMode
		trustee           trustee
	}

	trustee struct {
		multipleTrustee          *trustee
		multipleTrusteeOperation int32
		trusteeForm              trusteeForm
		trusteeType              trusteeType
		name                     uintptr
	}
)

const (
	accessMaskDesiredPermission accessMask = 0x12019f

	accessModeGrant accessMode = 1

	desiredAccessReadControl desiredAccess = 0x20000
	desiredAccessWriteDac    desiredAccess = 0x40000

	gvmga = "GrantVmGroupAccess:"

	inheritModeNoInheritance                  inheritMode = 0x0
	inheritModeSubContainersAndObjectsInherit inheritMode = 0x3

	objectTypeFileObject objectType = 0x1

	securityInformationDACL securityInformation = 0x4

	shareModeRead  shareMode = 0x1
	shareModeWrite shareMode = 0x2

	sidVmGroup                   = "S-1-5-83-1-3166535780-1122986932-343720105-43916321"
	sidVmWorkerProcessCapability = "S-1-15-3-1024-2268835264-3721307629-241982045-173645152-1490879176-104643441-2915960892-1612460704"

	trusteeFormIsSid trusteeForm = 0

	trusteeTypeWellKnownGroup trusteeType = 5
)

// GrantVMGroupAccess sets the DACL for a specified file or directory to
// include Grant ACE entries for both the VM Group SID, and the VM Worker Process
// Capability SID. This is a golang re-implementation of the same function in
// vmcompute, just not exported in RS5. Which kind of sucks. Sucks a lot :/
func GrantVmGroupAccess(name string) error {
	// Stat (to determine if `name` is a directory).
	s, err := os.Stat(name)
	if err != nil {
		return errors.Wrapf(err, "%s os.Stat %s", gvmga, name)
	}

	// Get a handle to the file/directory. Must defer Close on success.
	fd, err := createFile(name, s.IsDir())
	if err != nil {
		return err // Already wrapped
	}
	defer syscall.CloseHandle(fd)

	// Get the current DACL and Security Descriptor. Must defer LocalFree on success.
	ot := objectTypeFileObject
	si := securityInformationDACL
	sd := uintptr(0)
	origDACL := uintptr(0)
	if err := getSecurityInfo(fd, uint32(ot), uint32(si), nil, nil, &origDACL, nil, &sd); err != nil {
		return errors.Wrapf(err, "%s GetSecurityInfo %s", gvmga, name)
	}
	defer syscall.LocalFree((syscall.Handle)(unsafe.Pointer(sd)))

	// Just a very large comment in case you ever want to debug this... Shows how
	// to use a winio library function to examine the security descriptor, and
	// how to decode obtained SD. This example if from a VHD which is attached to a
	// B in Hyper-V. The only real thing of note is the ACE with the VM WP Capability SID
	// The other ACEs aren't particularly interesting. Except to note that the second
	// SID is the SID of the worker process. We don't do that in this code, rather
	// add the VM Group SID. To debug would need to add `fmt` to imports. Will also
	// need `//sys getSecurityDescriptorLength(sd uintptr) (len uint32) = advapi32.GetSecurityDescriptorLength`
	// defined and regenerated.
	//
	// >> sdByteArray := make([]byte, getSecurityDescriptorLength(sd))
	// >> copy(sdByteArray, (*[0xffff]byte)(unsafe.Pointer(sd))[:len(sdByteArray)])
	// >> sddl, err := winio.SecurityDescriptorToSddl(sdByteArray)
	// >> if err != nil {
	// >>     return err
	// >> }
	// >> fmt.Println("SDDL:", sddl)
	//
	// Example output (pretty-printed) - first one (or two potentially) are the interesting ones:
	// D:AI
	//  (A;;0x12019f;;;S-1-15-3-1024-2268835264-3721307629-241982045-173645152-1490879176-104643441-2915960892-1612460704)
	//  (A;;0x12019f;;;S-1-5-83-1-3166535780-1122986932-343720105-43916321)
	//  (A;ID;FA;;;BA)
	//  (A;ID;FA;;;SY)
	//  (A;ID;0x1301bf;;;AU)
	//  (A;ID;0x1200a9;;;BU)
	//
	// And what ICACLS on that file shows
	// S-1-15-3-1024-2268835264-3721307629-241982045-173645152-1490879176-104643441-2915960892-1612460704:(R,W)
	// NT VIRTUAL MACHINE\BCBD8064-6BB4-42EF-A9C0-7C14211C9E02:(R,W)
	// BUILTIN\Administrators:(I)(F)
	// NT AUTHORITY\SYSTEM:(I)(F)
	// NT AUTHORITY\Authenticated Users:(I)(M)
	// BUILTIN\Users:(I)(RX)
	//
	// Translating D:AI(A;;0x12019f;;;S-1-15-3-1024-2268835264-3721307629-241982045-173645152-1490879176-104643441-2915960892-1612460704)
	// https://docs.microsoft.com/en-us/windows/desktop/secauthz/security-descriptor-string-format
	//  - D:AI DACL:AutoInherited followed by ACEs, of which the first one is:
	//  - (A;;0x12019f;;;S-1-15-3-1024-2268835264-3721307629-241982045-173645152-1490879176-104643441-2915960892-1612460704)
	// Format is ace_type;ace_flags;rights;object_guid;inherit_object_guid;account_sid;(resource_attribute)
	// ace_type = Allowed
	// ace_flags = 0
	// rights = 0x12019f (exercise for reader to decode)
	// object_guid=blank
	// inherit_object_guid=blank
	// account_sid=VM Worker Process Capability SID
	//
	// Translating (A;;0x12019f;;;S-1-5-83-1-3166535780-1122986932-343720105-43916321)
	// is simples. It's the SID of the specific VMWP.

	// Generate a new DACL which is the current DACL with the required ACEs added.
	// Must defer LocalFree on success.
	newDACL, err := generateDACLWithAcesAdded(name, s.IsDir(), origDACL)
	if err != nil {
		return err // Already wrapped
	}
	defer syscall.LocalFree((syscall.Handle)(unsafe.Pointer(newDACL)))

	// And finally use SetSecurityInfo to apply the updated DACL.
	if err := setSecurityInfo(fd, uint32(ot), uint32(si), uintptr(0), uintptr(0), newDACL, uintptr(0)); err != nil {
		return errors.Wrapf(err, "%s SetSecurityInfo %s", gvmga, name)
	}

	return nil
}

// createFile is a helper function to call [Nt]CreateFile to get a handle to
// the file or directory.
func createFile(name string, isDir bool) (syscall.Handle, error) {
	namep := syscall.StringToUTF16(name)
	da := uint32(desiredAccessReadControl | desiredAccessWriteDac)
	sm := uint32(shareModeRead | shareModeWrite)
	fa := uint32(syscall.FILE_ATTRIBUTE_NORMAL)
	if isDir {
		fa = uint32(fa | syscall.FILE_FLAG_BACKUP_SEMANTICS)
	}
	fd, err := syscall.CreateFile(&namep[0], da, sm, nil, syscall.OPEN_EXISTING, fa, 0)
	if err != nil {
		return 0, errors.Wrapf(err, "%s syscall.CreateFile %s", gvmga, name)
	}
	return fd, nil
}

// generateDACLWithAcesAdded generates a new DACL with the two needed ACEs added.
// The caller is responsible for LocalFree of the returned DACL on success.
func generateDACLWithAcesAdded(name string, isDir bool, origDACL uintptr) (uintptr, error) {
	// Generate pointers to the SIDs based on the string SIDs
	sid1, err := syscall.StringToSid(sidVmGroup)
	if err != nil {
		return 0, errors.Wrapf(err, "%s syscall.StringToSid %s %s", gvmga, name, sidVmGroup)
	}
	sid2, err := syscall.StringToSid(sidVmWorkerProcessCapability)
	if err != nil {
		return 0, errors.Wrapf(err, "%s syscall.StringToSid %s %s", gvmga, name, sidVmWorkerProcessCapability)
	}

	inheritance := inheritModeNoInheritance
	if isDir {
		inheritance = inheritModeSubContainersAndObjectsInherit
	}

	eaArray := []explicitAccess{
		explicitAccess{
			accessPermissions: accessMaskDesiredPermission,
			accessMode:        accessModeGrant,
			inheritance:       inheritance,
			trustee: trustee{
				trusteeForm: trusteeFormIsSid,
				trusteeType: trusteeTypeWellKnownGroup,
				name:        uintptr(unsafe.Pointer(sid1)),
			},
		},
		explicitAccess{
			accessPermissions: accessMaskDesiredPermission,
			accessMode:        accessModeGrant,
			inheritance:       inheritance,
			trustee: trustee{
				trusteeForm: trusteeFormIsSid,
				trusteeType: trusteeTypeWellKnownGroup,
				name:        uintptr(unsafe.Pointer(sid2)),
			},
		},
	}

	modifiedDACL := uintptr(0)
	if err := setEntriesInAcl(uintptr(uint32(2)), uintptr(unsafe.Pointer(&eaArray[0])), origDACL, &modifiedDACL); err != nil {
		return 0, errors.Wrapf(err, "%s SetEntriesInAcl %s", gvmga, name)
	}

	return modifiedDACL, nil
}
