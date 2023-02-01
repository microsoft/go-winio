//go:build windows

package fs

import (
	"golang.org/x/sys/windows"

	"github.com/Microsoft/go-winio/internal/stringbuffer"
)

//go:generate go run github.com/Microsoft/go-winio/tools/mkwinsyscall -output zsyscall_windows.go fs.go

// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew
//sys CreateFile(name string, access AccessMask, mode FileShareMode, sa *syscall.SecurityAttributes, createmode FileCreationDisposition, attrs FileAttribute, templatefile windows.Handle) (handle windows.Handle, err error) [failretval==windows.InvalidHandle] = CreateFileW

const NullHandle windows.Handle = 0

// AccessMask defines standard, specific, and generic rights.
//
//	Bitmask:
//	 3 3 2 2 2 2 2 2 2 2 2 2 1 1 1 1 1 1 1 1 1 1
//	 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0 9 8 7 6 5 4 3 2 1 0
//	+---------------+---------------+-------------------------------+
//	|G|G|G|G|Resvd|A| StandardRights|         SpecificRights        |
//	|R|W|E|A|     |S|               |                               |
//	+-+-------------+---------------+-------------------------------+
//
//	GR     Generic Read
//	GW     Generic Write
//	GE     Generic Exectue
//	GA     Generic All
//	Resvd  Reserved
//	AS     Access Security System
//
// https://learn.microsoft.com/en-us/windows/win32/secauthz/access-mask
//
// https://learn.microsoft.com/en-us/windows/win32/secauthz/generic-access-rights
//
// https://learn.microsoft.com/en-us/windows/win32/fileio/file-access-rights-constants
type AccessMask uint32

//nolint:revive // SNAKE_CASE is not idiomatic in Go, but aligned with Win32 API.
const (
	// Not actually any.
	//
	// For CreateFile: "query certain metadata such as file, directory, or device attributes without accessing that file or device"
	// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew#parameters
	FILE_ANY_ACCESS AccessMask = 0

	// Specific Object Access
	// from ntioapi.h

	FILE_READ_DATA      AccessMask = (0x0001) // file & pipe
	FILE_LIST_DIRECTORY AccessMask = (0x0001) // directory

	FILE_WRITE_DATA AccessMask = (0x0002) // file & pipe
	FILE_ADD_FILE   AccessMask = (0x0002) // directory

	FILE_APPEND_DATA          AccessMask = (0x0004) // file
	FILE_ADD_SUBDIRECTORY     AccessMask = (0x0004) // directory
	FILE_CREATE_PIPE_INSTANCE AccessMask = (0x0004) // named pipe

	FILE_READ_EA         AccessMask = (0x0008) // file & directory
	FILE_READ_PROPERTIES AccessMask = FILE_READ_EA

	FILE_WRITE_EA         AccessMask = (0x0010) // file & directory
	FILE_WRITE_PROPERTIES AccessMask = FILE_WRITE_EA

	FILE_EXECUTE  AccessMask = (0x0020) // file
	FILE_TRAVERSE AccessMask = (0x0020) // directory

	FILE_DELETE_CHILD AccessMask = (0x0040) // directory

	FILE_READ_ATTRIBUTES AccessMask = (0x0080) // all

	FILE_WRITE_ATTRIBUTES AccessMask = (0x0100) // all

	FILE_ALL_ACCESS      AccessMask = (STANDARD_RIGHTS_REQUIRED | SYNCHRONIZE | 0x1FF)
	FILE_GENERIC_READ    AccessMask = (STANDARD_RIGHTS_READ | FILE_READ_DATA | FILE_READ_ATTRIBUTES | FILE_READ_EA | SYNCHRONIZE)
	FILE_GENERIC_WRITE   AccessMask = (STANDARD_RIGHTS_WRITE | FILE_WRITE_DATA | FILE_WRITE_ATTRIBUTES | FILE_WRITE_EA | FILE_APPEND_DATA | SYNCHRONIZE)
	FILE_GENERIC_EXECUTE AccessMask = (STANDARD_RIGHTS_EXECUTE | FILE_READ_ATTRIBUTES | FILE_EXECUTE | SYNCHRONIZE)

	SPECIFIC_RIGHTS_ALL AccessMask = 0x0000FFFF

	// Standard Access
	// from ntseapi.h

	DELETE       AccessMask = 0x0001_0000
	READ_CONTROL AccessMask = 0x0002_0000
	WRITE_DAC    AccessMask = 0x0004_0000
	WRITE_OWNER  AccessMask = 0x0008_0000
	SYNCHRONIZE  AccessMask = 0x0010_0000

	STANDARD_RIGHTS_REQUIRED AccessMask = 0x000F_0000

	STANDARD_RIGHTS_READ    AccessMask = READ_CONTROL
	STANDARD_RIGHTS_WRITE   AccessMask = READ_CONTROL
	STANDARD_RIGHTS_EXECUTE AccessMask = READ_CONTROL

	STANDARD_RIGHTS_ALL AccessMask = 0x001F_0000

	// Other Access
	// from ntseapi.h

	ACCESS_SYSTEM_SECURITY AccessMask = 0x0100_0000
	MAXIMUM_ALLOWED        AccessMask = 0x0200_0000

	GENERIC_READ    AccessMask = 0x8000_0000
	GENERIC_WRITE   AccessMask = 0x4000_0000
	GENERIC_EXECUTE AccessMask = 0x2000_0000
	GENERIC_ALL     AccessMask = 0x1000_0000
)

type FileShareMode uint32

//nolint:revive // SNAKE_CASE is not idiomatic in Go, but aligned with Win32 API.
const (
	FILE_SHARE_NONE        FileShareMode = 0x00
	FILE_SHARE_READ        FileShareMode = 0x01
	FILE_SHARE_WRITE       FileShareMode = 0x02
	FILE_SHARE_DELETE      FileShareMode = 0x04
	FILE_SHARE_VALID_FLAGS FileShareMode = 0x07
)

type FileCreationDisposition uint32

//nolint:revive // SNAKE_CASE is not idiomatic in Go, but aligned with Win32 API.
const (
	// from winbase.h

	CREATE_NEW        FileCreationDisposition = 0x01
	CREATE_ALWAYS     FileCreationDisposition = 0x02
	OPEN_EXISTING     FileCreationDisposition = 0x03
	OPEN_ALWAYS       FileCreationDisposition = 0x04
	TRUNCATE_EXISTING FileCreationDisposition = 0x05

	// for NtCreateFile
	// from ntioapi.h
	//
	// https://learn.microsoft.com/en-us/windows/win32/api/winternl/nf-winternl-ntcreatefile

	FILE_SUPERSEDE           FileCreationDisposition = 0x00
	FILE_OPEN                FileCreationDisposition = 0x01
	FILE_CREATE              FileCreationDisposition = 0x02
	FILE_OPEN_IF             FileCreationDisposition = 0x03
	FILE_OVERWRITE           FileCreationDisposition = 0x04
	FILE_OVERWRITE_IF        FileCreationDisposition = 0x05
	FILE_MAXIMUM_DISPOSITION FileCreationDisposition = 0x05
)

// https://learn.microsoft.com/en-us/windows/win32/fileio/file-attribute-constants
type FileAttribute uint32

//nolint:revive // SNAKE_CASE is not idiomatic in Go, but aligned with Win32 API.
const ( // from winnt.h
	FILE_ATTRIBUTE_READONLY            FileAttribute = 0x0000_0001
	FILE_ATTRIBUTE_HIDDEN              FileAttribute = 0x0000_0002
	FILE_ATTRIBUTE_SYSTEM              FileAttribute = 0x0000_0004
	FILE_ATTRIBUTE_DIRECTORY           FileAttribute = 0x0000_0010
	FILE_ATTRIBUTE_ARCHIVE             FileAttribute = 0x0000_0020
	FILE_ATTRIBUTE_DEVICE              FileAttribute = 0x0000_0040
	FILE_ATTRIBUTE_NORMAL              FileAttribute = 0x0000_0080
	FILE_ATTRIBUTE_TEMPORARY           FileAttribute = 0x0000_0100
	FILE_ATTRIBUTE_SPARSE_FILE         FileAttribute = 0x0000_0200
	FILE_ATTRIBUTE_REPARSE_POINT       FileAttribute = 0x0000_0400
	FILE_ATTRIBUTE_COMPRESSED          FileAttribute = 0x0000_0800
	FILE_ATTRIBUTE_OFFLINE             FileAttribute = 0x0000_1000
	FILE_ATTRIBUTE_NOT_CONTENT_INDEXED FileAttribute = 0x0000_2000
	FILE_ATTRIBUTE_ENCRYPTED           FileAttribute = 0x0000_4000
	FILE_ATTRIBUTE_INTEGRITY_STREAM    FileAttribute = 0x0000_8000
	FILE_ATTRIBUTE_VIRTUAL             FileAttribute = 0x0001_0000
	FILE_ATTRIBUTE_NO_SCRUB_DATA       FileAttribute = 0x0002_0000
)

// CreateFile and co. take flags or attributes together as one parameter.
// Define alias until we can use generics to allow both
type FileFlag = FileAttribute

//nolint:revive // SNAKE_CASE is not idiomatic in Go, but aligned with Win32 API.
const ( // from winnt.h
	FILE_FLAG_WRITE_THROUGH       FileFlag = 0x8000_0000
	FILE_FLAG_OVERLAPPED          FileFlag = 0x4000_0000
	FILE_FLAG_NO_BUFFERING        FileFlag = 0x2000_0000
	FILE_FLAG_RANDOM_ACCESS       FileFlag = 0x1000_0000
	FILE_FLAG_SEQUENTIAL_SCAN     FileFlag = 0x0800_0000
	FILE_FLAG_DELETE_ON_CLOSE     FileFlag = 0x0400_0000
	FILE_FLAG_BACKUP_SEMANTICS    FileFlag = 0x0200_0000
	FILE_FLAG_POSIX_SEMANTICS     FileFlag = 0x0100_0000
	FILE_FLAG_OPEN_REPARSE_POINT  FileFlag = 0x0020_0000
	FILE_FLAG_OPEN_NO_RECALL      FileFlag = 0x0010_0000
	FILE_FLAG_FIRST_PIPE_INSTANCE FileFlag = 0x0008_0000
)

type FileSQSFlag = FileAttribute

//nolint:revive // SNAKE_CASE is not idiomatic in Go, but aligned with Win32 API.
const ( // from winbase.h
	SECURITY_ANONYMOUS      FileSQSFlag = (SecurityAnonymous << 16)
	SECURITY_IDENTIFICATION FileSQSFlag = (SecurityIdentification << 16)
	SECURITY_IMPERSONATION  FileSQSFlag = (SecurityImpersonation << 16)
	SECURITY_DELEGATION     FileSQSFlag = (SecurityDelegation << 16)

	SECURITY_CONTEXT_TRACKING FileSQSFlag = 0x00040000
	SECURITY_EFFECTIVE_ONLY   FileSQSFlag = 0x00080000

	SECURITY_SQOS_PRESENT     FileSQSFlag = 0x00100000
	SECURITY_VALID_SQOS_FLAGS FileSQSFlag = 0x001F0000
)

// GetFinalPathNameByHandle flags
//
// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getfinalpathnamebyhandlew#parameters
type GetFinalPathFlag uint32

//nolint:revive // SNAKE_CASE is not idiomatic in Go, but aligned with Win32 API.
const (
	GetFinalPathDefaultFlag GetFinalPathFlag = 0x0

	FILE_NAME_NORMALIZED GetFinalPathFlag = 0x0
	FILE_NAME_OPENED     GetFinalPathFlag = 0x8

	VOLUME_NAME_DOS  GetFinalPathFlag = 0x0
	VOLUME_NAME_GUID GetFinalPathFlag = 0x1
	VOLUME_NAME_NT   GetFinalPathFlag = 0x2
	VOLUME_NAME_NONE GetFinalPathFlag = 0x4
)

// getFinalPathNameByHandle facilitates calling the Windows API GetFinalPathNameByHandle
// with the given handle and flags. It transparently takes care of creating a buffer of the
// correct size for the call.
//
// https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-getfinalpathnamebyhandlew
func GetFinalPathNameByHandle(h windows.Handle, flags GetFinalPathFlag) (string, error) {
	b := stringbuffer.NewWString()
	//TODO: can loop infinitely if Win32 keeps returning the same (or a larger) n?
	for {
		n, err := windows.GetFinalPathNameByHandle(h, b.Pointer(), b.Cap(), uint32(flags))
		if err != nil {
			return "", err
		}
		// If the buffer wasn't large enough, n will be the total size needed (including null terminator).
		// Resize and try again.
		if n > b.Cap() {
			b.ResizeTo(n)
			continue
		}
		// If the buffer is large enough, n will be the size not including the null terminator.
		// Convert to a Go string and return.
		return b.String(), nil
	}
}
