package sockets

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

// todo: replace this with generics
// The function calls should be:
//
//   type RawSockaddrHeader {
//   	Family uint16
//   }
//
//   func ConnectEx[T ~RawSockaddrHeader] (s Handle, a *T, ...) error {
//   	n := unsafe.SizeOf(*a)
//   	r1, _, e1 := syscall.Syscall9(connectExFunc.addr, 7, uintptr(s),
//  		uintptr(unsafe.Pointer(a)), uintptr(n), /* ... */)
//   	/* ... */
//   }
//
// Similarly, `GetAcceptExSockaddrs` requires a `**sockaddr`, so the syscall can change the pointer
// to data it allocates. Currently, the options are (1) dealing with pointers to the interface
// `* RawSockaddr`, use reflection or pull the pointer from the internal interface representation,
// and change where the interface points to; or (2) allocate dedicate, presized buffers based on
// `(r RawSockaddr).Sockaddr()`'s return, and pass that to `(r RawSockaddr).FromBytes()`.
// It would be safer and more readable to have:
//
//  	func GetAcceptExSockaddrs[L ~RawSockaddrHeader, R ~RawSockaddrHeader](
//  		b *byte,
//  		rxlen uint32,
//  		local **L,
//  		remote **R,
//  	) error { /*...*/ }

// RawSockaddr allows structs to be used with Bind and ConnectEx. The
// struct must meet the Wind32 sockaddr requirements specified here:
// https://docs.microsoft.com/en-us/windows/win32/winsock/sockaddr-2
type RawSockaddr interface {
	// Sockaddr returns a pointer to the RawSockaddr and the length of the struct.
	Sockaddr() (ptr unsafe.Pointer, len int32, err error)

	// FromBytes populates the RawsockAddr with the data in the byte array.
	// Implementers should check the buffer is correctly sized and the address family
	// is appropriate.
	// Receivers should be pointers.
	FromBytes([]byte) error
}

func validateSockAddr(ptr unsafe.Pointer, len int32) error {
	if ptr == nil {
		return fmt.Errorf("pointer is %p: %w", ptr, ErrInvalidPointer)
	}
	if len < 1 {
		return fmt.Errorf("buffer size %d < 1: %w", len, ErrBufferSize)
	}
	return nil
}
