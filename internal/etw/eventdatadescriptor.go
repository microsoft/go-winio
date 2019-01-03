package etw

import (
	"unsafe"
)

type eventDataDescriptorType uint8

const (
	eventDataDescriptorTypeUserData eventDataDescriptorType = iota
	eventDataDescriptorTypeEventMetadata
	eventDataDescriptorTypeProviderMetadata
)

type eventDataDescriptor struct {
	ptr       uint64
	size      uint32
	dataType  eventDataDescriptorType
	reserved1 uint8
	reserved2 uint16
}

func newEventDataDescriptor(dataType eventDataDescriptorType, buffer []byte) eventDataDescriptor {
	// Passing a pointer to Go-managed memory as part of a block of memory is
	// risky since the GC doesn't know about it. If we find a better way to do
	// this we should use it instead.
	return eventDataDescriptor{
		ptr:      uint64(uintptr(unsafe.Pointer(&buffer[0]))),
		size:     uint32(len(buffer)),
		dataType: dataType,
	}
}
