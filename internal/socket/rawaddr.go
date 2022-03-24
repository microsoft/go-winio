package socket

import (
	"errors"
	"fmt"
	"unsafe"
)

// todo: should these be custom types to store the desired/actual size and addr family?

var (
	ErrBufferSize     = errors.New("buffer size")
	ErrInvalidPointer = errors.New("invalid pointer")
	ErrAddrFamily     = errors.New("address family")
)

// todo: helsaawy - replace this with generics, along with GetSockName and co.

// RawSockaddr allows structs to be used with Bind and ConnectEx. The
// struct must meet the Win32 sockaddr requirements specified here:
// https://docs.microsoft.com/en-us/windows/win32/winsock/sockaddr-2
type RawSockaddr interface {
	// Sockaddr returns a pointer to the RawSockaddr and the length of the struct.
	Sockaddr() (unsafe.Pointer, int32, error)

	// FromBytes populates the RawsockAddr with the data in the byte array.
	// Implementers should check the buffer is correctly sized and the address family
	// is appropriate.
	// Receivers should be pointers.
	FromBytes([]byte) error
}

func validateSockAddr(ptr unsafe.Pointer, n int32) error {
	if ptr == nil {
		return fmt.Errorf("pointer is %p: %w", ptr, ErrInvalidPointer)
	}
	if n < 1 {
		return fmt.Errorf("buffer size %d < 1: %w", n, ErrBufferSize)
	}
	return nil
}
